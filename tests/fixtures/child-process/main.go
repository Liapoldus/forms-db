package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	sdkinfra "github.com/Liapoldus/plugin-sdk/infrastructure"
	"github.com/Liapoldus/pluginprotocol/presentation/peer"
)

const generation = "forms-child-generation-1"

type identity struct {
	certificate    tls.Certificate
	certificatePEM []byte
	keyPEM         []byte
}

type identities struct {
	root       *x509.Certificate
	rootKey    *ecdsa.PrivateKey
	rootPEM    []byte
	coreServer identity
	coreClient identity
	plugin     identity
	peerPlugin identity
	caller     identity
	serverREST identity
	outsider   identity
}

type grantState struct {
	mu     sync.Mutex
	next   int
	issued map[string][]byte
	dsn    []byte
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	contract, err := sdkinfra.LoadHTTPContract()
	if err != nil {
		return err
	}
	issued, err := createIdentities()
	if err != nil {
		return err
	}
	directory, err := os.MkdirTemp("", "forms-child-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)
	dsn := filepath.Join(directory, "forms.sqlite")
	files := map[string][]byte{
		"root.pem":            issued.rootPEM,
		"plugin.pem":          issued.plugin.certificatePEM,
		"plugin-key.pem":      issued.plugin.keyPEM,
		"peer-plugin.pem":     issued.peerPlugin.certificatePEM,
		"peer-plugin-key.pem": issued.peerPlugin.keyPEM,
	}
	crl, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number: big.NewInt(1), ThisUpdate: time.Now().UTC().Add(-time.Minute), NextUpdate: time.Now().UTC().Add(time.Hour),
	}, issued.root, issued.rootKey)
	if err != nil {
		return err
	}
	files["revocation.pem"] = pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: crl})
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(directory, name), contents, 0o600); err != nil {
			return err
		}
	}
	raw := []byte(`{"driver":"sqlite","dsn":"secret:forms-child-dsn","cursorSecretRef":"secret:forms-child-cursor","tablePrefix":"forms_child_","schemas":{"contact":{"type":"object","properties":{"email":{"type":"string"}},"required":["email"],"additionalProperties":false}}}`)
	digest := sha256.Sum256(raw)
	digestHex := hex.EncodeToString(digest[:])
	grants := &grantState{issued: map[string][]byte{}, dsn: []byte(dsn)}
	coreMux := http.NewServeMux()
	coreMux.HandleFunc(contract.Core.ConfigPull.Method+" "+contract.Core.ConfigPull.PathTemplate, func(writer http.ResponseWriter, request *http.Request) {
		if request.PathValue("generation") != generation {
			http.NotFound(writer, request)
			return
		}
		headers := contract.Core.ConfigPull.ResponseHeaders
		writer.Header().Set("content-type", contract.Core.ConfigPull.ResponseMediaType)
		writer.Header().Set(headers["generation"], generation)
		writer.Header().Set(headers["schemaVersion"], "1")
		writer.Header().Set(headers["sha256"], digestHex)
		writer.Header().Set(headers["generationState"], "active")
		_, _ = writer.Write(raw)
	})
	coreMux.HandleFunc(contract.Core.SecretGrant.Issue.Method+" "+contract.Core.SecretGrant.Issue.PathTemplate, func(writer http.ResponseWriter, request *http.Request) {
		var query sdkmodels.SecretGrantRequest
		if json.NewDecoder(io.LimitReader(request.Body, 4096)).Decode(&query) != nil || query.Generation != generation ||
			(query.Reference != "secret:forms-child-dsn" && query.Reference != "secret:forms-child-cursor") || query.Purpose == "" {
			http.Error(writer, "grant refused", http.StatusForbidden)
			return
		}
		grants.mu.Lock()
		grants.next++
		handle := fmt.Sprintf("forms-grant-%d", grants.next)
		if query.Reference == "secret:forms-child-dsn" {
			grants.issued[handle] = grants.dsn
		} else {
			grants.issued[handle] = bytes.Repeat([]byte("k"), 32)
		}
		grants.mu.Unlock()
		writer.Header().Set("content-type", contract.Core.SecretGrant.Issue.ResponseMediaType)
		_ = json.NewEncoder(writer).Encode(sdkmodels.SecretGrant{Handle: handle, Reference: query.Reference, Purpose: query.Purpose, Generation: query.Generation, ExpiresAt: time.Now().Add(time.Minute)})
	})
	coreMux.HandleFunc(contract.Core.SecretGrant.Redemption.Method+" "+contract.Core.SecretGrant.Redemption.PathTemplate, func(writer http.ResponseWriter, request *http.Request) {
		handle := request.PathValue("handle")
		var query sdkmodels.SecretRedemption
		if json.NewDecoder(io.LimitReader(request.Body, 4096)).Decode(&query) != nil || query.Handle != handle {
			http.Error(writer, "grant refused", http.StatusForbidden)
			return
		}
		grants.mu.Lock()
		value, allowed := grants.issued[handle]
		delete(grants.issued, handle)
		grants.mu.Unlock()
		if !allowed {
			http.Error(writer, "grant spent", http.StatusForbidden)
			return
		}
		writer.Header().Set("content-type", contract.Core.SecretGrant.Redemption.ResponseMediaType)
		_, _ = writer.Write(value)
	})
	coreProvider, err := credentialProvider(contract, issued.rootPEM, issued.coreServer, issued.coreClient)
	if err != nil {
		return err
	}
	revocation, err := sdkinfra.NewRevocation(sdkinfra.RevocationConfiguration{
		Authorities: []*x509.Certificate{issued.root}, Bundles: [][]byte{crl},
	})
	if err != nil {
		return err
	}
	coreServer, err := sdkinfra.NewMutualTLSServer(contract, sdkinfra.MutualTLSServerConfig{
		Handler: coreMux, Provider: coreProvider,
		Peer:       sdkmodels.PeerIdentity{CommonName: issued.plugin.certificate.Leaf.Subject.CommonName},
		Revocation: revocation,
	})
	if err != nil {
		return err
	}
	coreListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	go func() { _ = coreServer.Serve(coreListener) }()
	defer func() { _ = coreServer.GracefulShutdown(context.Background()) }()
	restAddress, err := freeAddress()
	if err != nil {
		return err
	}
	peerAddress, err := freeAddress()
	if err != nil {
		return err
	}
	binary := filepath.Join(directory, "forms-db")
	build := exec.Command("go", "build", "-o", binary, "./cmd/forms-db")
	build.Dir = repositoryRoot()
	build.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.26.0")
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build child: %w: %s", err, output)
	}
	pluginCredentials, err := credentialProvider(contract, issued.rootPEM, issued.coreClient, issued.coreClient)
	if err != nil {
		return err
	}
	pluginHTTP, err := sdkinfra.NewMutualTLSClient(contract, pluginCredentials, sdkinfra.MutualTLSClientConfig{
		Peer:       sdkmodels.PeerIdentity{CommonName: issued.plugin.certificate.Leaf.Subject.CommonName},
		ServerName: "localhost", Revocation: revocation,
	})
	if err != nil {
		return err
	}
	defer pluginHTTP.CloseIdleConnections()
	pluginClient, err := sdkinfra.NewPluginClient(contract, "https://"+restAddress, pluginHTTP,
		sdkmodels.PeerIdentity{CommonName: issued.plugin.certificate.Leaf.Subject.CommonName})
	if err != nil {
		return err
	}
	start := func() (*exec.Cmd, *bytes.Buffer, error) {
		child := exec.Command(binary,
			"--instance-id=forms-child", "--replica-id=forms-replica",
			"--rest-listen="+restAddress, "--core-url=https://"+coreListener.Addr().String(),
			"--core-server-name=localhost", "--core-common-name="+issued.coreServer.certificate.Leaf.Subject.CommonName,
			"--core-client-common-name="+issued.coreClient.certificate.Leaf.Subject.CommonName,
			"--ca-file="+filepath.Join(directory, "root.pem"),
			"--server-cert="+filepath.Join(directory, "plugin.pem"), "--server-key="+filepath.Join(directory, "plugin-key.pem"),
			"--client-cert="+filepath.Join(directory, "plugin.pem"), "--client-key="+filepath.Join(directory, "plugin-key.pem"),
			"--crl-file="+filepath.Join(directory, "revocation.pem"),
			"--peer-listen="+peerAddress, "--peer-identity=spiffe://liapoldus.test/forms",
			"--peer-allowed-caller=spiffe://liapoldus.test/server",
			"--peer-ca-file="+filepath.Join(directory, "root.pem"),
			"--peer-cert="+filepath.Join(directory, "peer-plugin.pem"), "--peer-key="+filepath.Join(directory, "peer-plugin-key.pem"),
			"--peer-carrier=tcp")
		child.Dir = repositoryRoot()
		logs := new(bytes.Buffer)
		child.Stdout, child.Stderr = logs, logs
		return child, logs, child.Start()
	}
	stop := func(child *exec.Cmd) error {
		if child == nil || child.Process == nil {
			return nil
		}
		_ = child.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- child.Wait() }()
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			_ = child.Process.Kill()
			<-done
			return errors.New("child stop timed out")
		}
	}
	reload := func() (*exec.Cmd, error) {
		child, _, err := start()
		if err != nil {
			return nil, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var healthErr error
		for ctx.Err() == nil {
			healthErr = pluginClient.Health(ctx)
			if healthErr == nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if healthErr != nil {
			_ = stop(child)
			return nil, fmt.Errorf("child health: %w", healthErr)
		}
		ack, err := pluginClient.Reload(ctx, sdkmodels.Reload{Generation: generation, SHA256: digestHex, SchemaVersion: "1"})
		if err != nil || !ack.Applied {
			_ = stop(child)
			return nil, fmt.Errorf("child reload: %w", err)
		}
		ready, err := pluginClient.Readiness(ctx)
		if err != nil || !ready.Ready || ready.Generation != generation {
			_ = stop(child)
			return nil, fmt.Errorf("child readiness: %w", err)
		}
		return child, nil
	}
	child, err := reload()
	if err != nil {
		return err
	}
	defer func() { _ = stop(child) }()
	callerRegistry, err := peer.NewRegistry().Build()
	if err != nil {
		return err
	}
	call := func(method string, payload []byte) (int, string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		roots := x509.NewCertPool()
		roots.AddCert(issued.root)
		client, err := peer.Dial(ctx, peer.ClientConfig{
			Network: peer.NetworkConfig{Carrier: peer.CarrierTCP, Endpoint: peerAddress},
			Security: peer.SecurityConfig{Identity: "spiffe://liapoldus.test/server", Certificate: issued.caller.certificate,
				Roots: roots, PeerIdentity: "spiffe://liapoldus.test/forms"}, Handler: callerRegistry,
		})
		if err != nil {
			return 0, "", err
		}
		defer client.Close()
		result, err := client.Call(ctx, peer.Method(method), payload)
		if err != nil {
			return 0, "", err
		}
		var response struct {
			Status int    `json:"status"`
			Body   string `json:"body"`
		}
		if err := json.Unmarshal(result.Payload, &response); err != nil {
			return 0, "", err
		}
		return response.Status, response.Body, nil
	}
	status, body, err := call("forms.submit", []byte(`{"site":"smoke","schemaName":"contact","data":{"email":"persist@example.test"}}`))
	if err != nil || status != 200 {
		return fmt.Errorf("submit failed: %v / %d", err, status)
	}
	var submitted struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(body), &submitted) != nil || submitted.ID == "" {
		return errors.New("submit returned no ID")
	}
	if err := stop(child); err != nil {
		return err
	}
	child = nil
	child, err = reload()
	if err != nil {
		return err
	}
	status, body, err = call("forms.list", []byte(`{"site":"smoke","schemaName":"contact","limit":10}`))
	if err != nil || status != 200 {
		return fmt.Errorf("list after restart failed: %v / %d", err, status)
	}
	var listed struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if json.Unmarshal([]byte(body), &listed) != nil || len(listed.Items) != 1 || listed.Items[0].ID != submitted.ID {
		return errors.New("SQLite record was not recovered after child restart")
	}
	if err := runServerToForms(directory, issued, peerAddress, contract); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	roots := x509.NewCertPool()
	roots.AddCert(issued.root)
	outsider, err := peer.Dial(ctx, peer.ClientConfig{
		Network: peer.NetworkConfig{Carrier: peer.CarrierTCP, Endpoint: peerAddress},
		Security: peer.SecurityConfig{Identity: "spiffe://liapoldus.test/outsider", Certificate: issued.outsider.certificate,
			Roots: roots, PeerIdentity: "spiffe://liapoldus.test/forms"}, Handler: callerRegistry,
	})
	if err != nil {
		return err
	}
	defer outsider.Close()
	_, err = outsider.Call(ctx, peer.Method("forms.submit"), []byte(`{"site":"smoke","schemaName":"contact","data":{"email":"not-saved@example.test"}}`))
	if !errors.Is(err, peer.ErrUnauthorized) {
		return errors.New("unauthorized caller was not rejected")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]bool{"reload": true, "peerSubmit": true, "sqliteRestart": true, "serverToForms": true, "unauthorizedCallerRejected": true})
}

func freeAddress() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	address := listener.Addr().String()
	return address, listener.Close()
}

func repositoryRoot() string {
	_, current, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(current), "..", "..", ".."))
}

func credentialProvider(contract sdkinfra.HTTPContract, roots []byte, server, client identity) (*sdkinfra.StaticCredentialsProvider, error) {
	material, err := sdkinfra.LoadCredentials(contract, sdkinfra.CredentialsMaterial{
		CABundle: roots, ServerCertificatePEM: server.certificatePEM, ServerKeyPEM: server.keyPEM,
		ClientCertificatePEM: client.certificatePEM, ClientKeyPEM: client.keyPEM,
	})
	if err != nil {
		return nil, err
	}
	return sdkinfra.NewStaticCredentialsProvider(material)
}

func createIdentities() (identities, error) {
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return identities{}, err
	}
	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test root"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		return identities{}, err
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		return identities{}, err
	}
	issue := func(serial int64, name, uri string) (identity, error) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return identity{}, err
		}
		template := &x509.Certificate{
			SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name},
			DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
			NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
			KeyUsage:    x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		}
		if uri != "" {
			parsed, err := url.Parse(uri)
			if err != nil {
				return identity{}, err
			}
			template.URIs = []*url.URL{parsed}
		}
		der, err := x509.CreateCertificate(rand.Reader, template, root, &key.PublicKey, rootKey)
		if err != nil {
			return identity{}, err
		}
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		keyDER, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return identity{}, err
		}
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
		pair, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return identity{}, err
		}
		return identity{certificate: pair, certificatePEM: certPEM, keyPEM: keyPEM}, nil
	}
	coreServer, err := issue(2, "test Core server", "")
	if err != nil {
		return identities{}, err
	}
	coreClient, err := issue(3, "test Core client", "")
	if err != nil {
		return identities{}, err
	}
	plugin, err := issue(4, "test forms replica", "")
	if err != nil {
		return identities{}, err
	}
	peerPlugin, err := issue(5, "test forms peer", "spiffe://liapoldus.test/forms")
	if err != nil {
		return identities{}, err
	}
	caller, err := issue(6, "test server caller", "spiffe://liapoldus.test/server")
	if err != nil {
		return identities{}, err
	}
	serverREST, err := issue(7, "test server replica", "")
	if err != nil {
		return identities{}, err
	}
	outsider, err := issue(8, "test unauthorized caller", "spiffe://liapoldus.test/outsider")
	if err != nil {
		return identities{}, err
	}
	return identities{root: root, rootKey: rootKey, rootPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER}),
		coreServer: coreServer, coreClient: coreClient, plugin: plugin, peerPlugin: peerPlugin, caller: caller, serverREST: serverREST, outsider: outsider}, nil
}
