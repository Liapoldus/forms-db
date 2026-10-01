package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	sdkinfra "github.com/Liapoldus/plugin-sdk/infrastructure"
)

func runServerToForms(directory string, issued identities, formsAddress string, contract sdkinfra.HTTPContract) error {
	const serverGeneration = "server-child-generation-1"
	publicAddress, err := freeAddress()
	if err != nil {
		return err
	}
	restAddress, err := freeAddress()
	if err != nil {
		return err
	}
	settings, err := json.Marshal(map[string]any{
		"schemaVersion": 1,
		"config": map[string]any{
			"listeners": []any{map[string]any{
				"id": "web", "kind": "http", "address": publicAddress,
				"hostnames": []string{}, "protocols": []string{"http1"}, "tls": map[string]any{"mode": "disabled"},
			}},
			"routes": []any{map[string]any{
				"id": "form", "listenerId": "web",
				"match":   map[string]any{"path": map[string]any{"type": "exact", "value": "/submit"}},
				"handler": map[string]any{"type": "plugin", "instanceId": "forms", "capability": "forms.submit", "mode": "call"},
			}},
		},
	})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(settings)
	digestHex := hex.EncodeToString(digest[:])
	mux := http.NewServeMux()
	mux.HandleFunc(contract.Core.ConfigPull.Method+" "+contract.Core.ConfigPull.PathTemplate, func(writer http.ResponseWriter, request *http.Request) {
		if request.PathValue("generation") != serverGeneration {
			http.NotFound(writer, request)
			return
		}
		headers := contract.Core.ConfigPull.ResponseHeaders
		writer.Header().Set("content-type", contract.Core.ConfigPull.ResponseMediaType)
		writer.Header().Set(headers["generation"], serverGeneration)
		writer.Header().Set(headers["schemaVersion"], "1")
		writer.Header().Set(headers["sha256"], digestHex)
		writer.Header().Set(headers["generationState"], "active")
		_, _ = writer.Write(settings)
	})
	provider, err := credentialProvider(contract, issued.rootPEM, issued.coreServer, issued.coreClient)
	if err != nil {
		return err
	}
	crl, err := os.ReadFile(filepath.Join(directory, "revocation.pem"))
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
		Handler: mux, Provider: provider,
		Peer:       sdkmodels.PeerIdentity{CommonName: issued.serverREST.certificate.Leaf.Subject.CommonName},
		Revocation: revocation,
	})
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	go func() { _ = coreServer.Serve(listener) }()
	defer func() { _ = coreServer.GracefulShutdown(context.Background()) }()
	for name, value := range map[string][]byte{
		"server-rest.pem":     issued.serverREST.certificatePEM,
		"server-rest-key.pem": issued.serverREST.keyPEM,
		"server-peer.pem":     issued.caller.certificatePEM,
		"server-peer-key.pem": issued.caller.keyPEM,
	} {
		if err := os.WriteFile(filepath.Join(directory, name), value, 0o600); err != nil {
			return err
		}
	}
	root := filepath.Clean(filepath.Join(repositoryRoot(), "..", "server"))
	binary := filepath.Join(directory, "server")
	build := exec.Command("go", "build", "-o", binary, "./cmd/server")
	build.Dir = root
	build.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.26.0")
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build Server child: %w: %s", err, output)
	}
	child := exec.Command(binary,
		"--instance-id=server-child", "--replica-id=server-replica", "--rest-listen="+restAddress,
		"--core-url=https://"+listener.Addr().String(), "--core-server-name=localhost",
		"--core-common-name="+issued.coreServer.certificate.Leaf.Subject.CommonName,
		"--core-client-common-name="+issued.coreClient.certificate.Leaf.Subject.CommonName,
		"--ca-file="+filepath.Join(directory, "root.pem"),
		"--server-cert="+filepath.Join(directory, "server-rest.pem"), "--server-key="+filepath.Join(directory, "server-rest-key.pem"),
		"--client-cert="+filepath.Join(directory, "server-rest.pem"), "--client-key="+filepath.Join(directory, "server-rest-key.pem"),
		"--crl-file="+filepath.Join(directory, "revocation.pem"),
		"--peer-target-id=forms", "--peer-endpoint="+formsAddress,
		"--peer-identity=spiffe://liapoldus.test/server", "--peer-expected-identity=spiffe://liapoldus.test/forms",
		"--peer-ca-file="+filepath.Join(directory, "root.pem"),
		"--peer-cert="+filepath.Join(directory, "server-peer.pem"), "--peer-key="+filepath.Join(directory, "server-peer-key.pem"),
	)
	child.Dir = root
	var logs bytes.Buffer
	child.Stdout, child.Stderr = &logs, &logs
	if err := child.Start(); err != nil {
		return err
	}
	defer func() {
		_ = child.Process.Signal(syscall.SIGTERM)
		finished := make(chan struct{})
		go func() { _ = child.Wait(); close(finished) }()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			_ = child.Process.Kill()
			<-finished
		}
	}()
	pluginProvider, err := credentialProvider(contract, issued.rootPEM, issued.coreClient, issued.coreClient)
	if err != nil {
		return err
	}
	clientTLS, err := sdkinfra.NewMutualTLSClient(contract, pluginProvider, sdkinfra.MutualTLSClientConfig{
		Peer:       sdkmodels.PeerIdentity{CommonName: issued.serverREST.certificate.Leaf.Subject.CommonName},
		ServerName: "localhost", Revocation: revocation,
	})
	if err != nil {
		return err
	}
	defer clientTLS.CloseIdleConnections()
	client, err := sdkinfra.NewPluginClient(contract, "https://"+restAddress, clientTLS,
		sdkmodels.PeerIdentity{CommonName: issued.serverREST.certificate.Leaf.Subject.CommonName})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for {
		if err := client.Health(ctx); err == nil {
			break
		}
		if ctx.Err() != nil {
			return errors.New("Server child did not become healthy")
		}
		time.Sleep(50 * time.Millisecond)
	}
	ack, err := client.Reload(ctx, sdkmodels.Reload{Generation: serverGeneration, SHA256: digestHex, SchemaVersion: "1"})
	if err != nil || !ack.Applied {
		return fmt.Errorf("Server child did not apply generation: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+publicAddress+"/submit",
		bytes.NewBufferString(`{"site":"smoke","schemaName":"contact","data":{"email":"from-server@example.test"}}`))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Server→forms request failed with HTTP %d", response.StatusCode)
	}
	var submission struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(contents, &submission) != nil || submission.ID == "" {
		return errors.New("Server→forms response omitted submission ID")
	}
	return nil
}
