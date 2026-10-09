package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/Liapoldus/forms-db/tests/fixtures/support"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	sdkinfra "github.com/Liapoldus/plugin-sdk/infrastructure"
	"github.com/Liapoldus/pluginprotocol/v2/presentation/peer"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const currentGeneration = "forms-shared-generation-1"
const candidateGeneration = "forms-shared-generation-2"

type identity struct {
	pair tls.Certificate
	cert []byte
	key  []byte
}

type grant struct {
	value      []byte
	generation string
	reference  string
	purpose    string
}

type grants struct {
	mu     sync.Mutex
	issued map[string]grant
	next   int
}

type response struct {
	status int
	body   string
}

func main() {
	if err := run(); err != nil {
		support.Written(fmt.Fprintln(os.Stderr, err))
		os.Exit(1)
	}
}

func run() error {
	contract, err := sdkinfra.LoadHTTPContract()
	if err != nil {
		return errors.New("load Plugin SDK HTTP contract")
	}
	directory, err := os.MkdirTemp("", "forms-multi-replica-")
	if err != nil {
		return errors.New("create temporary test directory")
	}
	defer func() { support.Check(os.RemoveAll(directory)) }()
	driver := os.Getenv("FORMS_MULTI_REPLICA_SQL_DRIVER")
	if driver == "" {
		driver = "sqlite"
	}
	if driver != "sqlite" && driver != "mysql" && driver != "postgres" {
		return errors.New("invalid shared replica database driver")
	}
	databaseDSN := os.Getenv("FORMS_MULTI_REPLICA_SQL_DSN")
	if driver == "sqlite" {
		databaseDSN = "file:" + filepath.ToSlash(filepath.Join(directory, "shared.sqlite")) + "?mode=rwc"
	} else if databaseDSN == "" {
		return errors.New("shared replica SQL DSN is required")
	}
	var prefixEntropy [4]byte
	if _, err := rand.Read(prefixEntropy[:]); err != nil {
		return errors.New("create unique shared replica storage prefix")
	}
	tablePrefix := "fr_" + hex.EncodeToString(prefixEntropy[:])
	if driver != "sqlite" {
		defer func() { support.Check(dropSharedTables(driver, databaseDSN, tablePrefix)) }()
	}
	identities, err := issueIdentities()
	if err != nil {
		return errors.New("issue mTLS fixture identities")
	}
	crl, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{
		Number: big.NewInt(1), ThisUpdate: time.Now().Add(-time.Minute), NextUpdate: time.Now().Add(time.Hour),
	}, identities.root, identities.rootKey)
	if err != nil {
		return errors.New("create empty test CRL")
	}
	files := map[string][]byte{
		"root.pem": identities.rootPEM,
		"core.pem": identities.coreServer.cert, "core-key.pem": identities.coreServer.key,
		"core-client.pem": identities.coreClient.cert, "core-client-key.pem": identities.coreClient.key,
		"replica-one.pem": identities.replicaOne.cert, "replica-one-key.pem": identities.replicaOne.key,
		"replica-two.pem": identities.replicaTwo.cert, "replica-two-key.pem": identities.replicaTwo.key,
		"replica-one-peer.pem": identities.replicaOnePeer.cert, "replica-one-peer-key.pem": identities.replicaOnePeer.key,
		"replica-two-peer.pem": identities.replicaTwoPeer.cert, "replica-two-peer-key.pem": identities.replicaTwoPeer.key,
		"server-caller.pem": identities.caller.cert, "server-caller-key.pem": identities.caller.key,
		"revocation.pem": pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: crl}),
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(directory, name), contents, 0o600); err != nil {
			return errors.New("write mTLS fixture material")
		}
	}
	firstSettings, err := settings("secret:forms-dsn", false, driver, tablePrefix)
	if err != nil {
		return errors.New("encode initial plugin settings")
	}
	secondSettings, err := settings("secret:forms-dsn", true, driver, tablePrefix)
	if err != nil {
		return errors.New("encode candidate plugin settings")
	}
	firstDigest := digest(firstSettings)
	secondDigest := digest(secondSettings)
	coreMux := http.NewServeMux()
	coreMux.HandleFunc(contract.Core.ConfigPull.Method+" "+contract.Core.ConfigPull.PathTemplate, func(writer http.ResponseWriter, request *http.Request) {
		generation := request.PathValue("generation")
		contents, hash := firstSettings, firstDigest
		if generation == candidateGeneration {
			contents, hash = secondSettings, secondDigest
		} else if generation != currentGeneration {
			http.NotFound(writer, request)
			return
		}
		headers := contract.Core.ConfigPull.ResponseHeaders
		writer.Header().Set("content-type", contract.Core.ConfigPull.ResponseMediaType)
		writer.Header().Set(headers["generation"], generation)
		writer.Header().Set(headers["schemaVersion"], "1")
		writer.Header().Set(headers["sha256"], hash)
		writer.Header().Set(headers["generationState"], "active")
		support.Written(writer.Write(contents))
	})
	grantState := &grants{issued: make(map[string]grant)}
	secretValues := map[string][]byte{
		"secret:forms-dsn": []byte(databaseDSN),
		"secret:cursor":    []byte(strings.Repeat("r", 32)),
	}
	coreMux.HandleFunc(contract.Core.SecretGrant.Issue.Method+" "+contract.Core.SecretGrant.Issue.PathTemplate, func(writer http.ResponseWriter, request *http.Request) {
		var asked sdkmodels.SecretGrantRequest
		if json.NewDecoder(http.MaxBytesReader(writer, request.Body, 4096)).Decode(&asked) != nil || asked.Validate() != nil {
			http.Error(writer, "grant refused", http.StatusForbidden)
			return
		}
		value, ok := secretValues[asked.Reference]
		if !ok || (asked.Generation != currentGeneration && asked.Generation != candidateGeneration) {
			http.Error(writer, "grant refused", http.StatusForbidden)
			return
		}
		grantState.mu.Lock()
		grantState.next++
		handle := fmt.Sprintf("grant-%d", grantState.next)
		grantState.issued[handle] = grant{value: value, generation: asked.Generation, reference: asked.Reference, purpose: asked.Purpose}
		grantState.mu.Unlock()
		writer.Header().Set("content-type", contract.Core.SecretGrant.Issue.ResponseMediaType)
		support.Check(json.NewEncoder(writer).Encode(sdkmodels.SecretGrant{
			Handle: handle, Reference: asked.Reference, Purpose: asked.Purpose,
			Generation: asked.Generation, ExpiresAt: time.Now().Add(time.Minute),
		}))
	})
	coreMux.HandleFunc(contract.Core.SecretGrant.Redemption.Method+" "+contract.Core.SecretGrant.Redemption.PathTemplate, func(writer http.ResponseWriter, request *http.Request) {
		handle := request.PathValue("handle")
		var redemption sdkmodels.SecretRedemption
		if json.NewDecoder(http.MaxBytesReader(writer, request.Body, 4096)).Decode(&redemption) != nil || redemption.Validate() != nil || redemption.Handle != handle {
			http.Error(writer, "grant refused", http.StatusForbidden)
			return
		}
		grantState.mu.Lock()
		issued, ok := grantState.issued[handle]
		delete(grantState.issued, handle)
		grantState.mu.Unlock()
		if !ok {
			http.Error(writer, "grant spent", http.StatusForbidden)
			return
		}
		writer.Header().Set("content-type", contract.Core.SecretGrant.Redemption.ResponseMediaType)
		support.Written(writer.Write(issued.value))
	})
	coreCredentials, err := credentials(contract, identities.rootPEM, identities.coreServer, identities.coreClient)
	if err != nil {
		return errors.New("prepare fake Core TLS credentials")
	}
	revocation, err := sdkinfra.NewRevocation(sdkinfra.RevocationConfiguration{
		Authorities: []*x509.Certificate{identities.root}, Bundles: [][]byte{pem.EncodeToMemory(&pem.Block{Type: "X509 CRL", Bytes: crl})},
	})
	if err != nil {
		return errors.New("prepare Core CRL verifier")
	}
	coreServer, err := sdkinfra.NewMutualTLSServer(contract, sdkinfra.MutualTLSServerConfig{
		Handler: coreMux, Provider: coreCredentials, Peer: sdkmodels.PeerIdentity{CommonName: "forms-replica"},
		Revocation: revocation,
	})
	if err != nil {
		return errors.New("create fake Core mTLS server")
	}
	coreListener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return errors.New("listen for fake Core")
	}
	go func() { support.Served(coreServer.Serve(coreListener)) }()
	defer func() { support.Check(coreServer.GracefulShutdown(context.Background())) }()

	binary := filepath.Join(directory, "forms-db")
	// #nosec G204 -- The fixture builds this binary in a private directory and passes arguments directly, without a shell.
	build := exec.CommandContext(context.Background(), "go", "build", "-o", binary, "./cmd/forms-db")
	build.Dir = repositoryRoot()
	build.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.26.0")
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build forms-db child: %w: %s", err, output)
	}
	restOne, err := freeAddress()
	if err != nil {
		return errors.New("allocate first REST endpoint")
	}
	restTwo, err := freeAddress()
	if err != nil {
		return errors.New("allocate second REST endpoint")
	}
	peerOne, err := freeAddress()
	if err != nil {
		return errors.New("allocate first peer endpoint")
	}
	peerTwo, err := freeAddress()
	if err != nil {
		return errors.New("allocate second peer endpoint")
	}
	coreURL := "https://" + coreListener.Addr().String()
	start := func(replica, rest, peerAddress, certName, peerCertName, peerIdentity string) (*exec.Cmd, error) {
		// #nosec G204 -- The fixture builds this binary in a private directory and passes arguments directly, without a shell.
		child := exec.CommandContext(context.Background(), binary,
			"--instance-id=forms-shared", "--replica-id="+replica, "--rest-listen="+rest,
			"--core-url="+coreURL, "--core-server-name=localhost",
			"--core-common-name=core-server", "--core-client-common-name=core-client",
			"--ca-file="+filepath.Join(directory, "root.pem"),
			"--server-cert="+filepath.Join(directory, certName+".pem"), "--server-key="+filepath.Join(directory, certName+"-key.pem"),
			"--client-cert="+filepath.Join(directory, certName+".pem"), "--client-key="+filepath.Join(directory, certName+"-key.pem"),
			"--crl-file="+filepath.Join(directory, "revocation.pem"),
			"--peer-listen="+peerAddress, "--peer-identity="+peerIdentity,
			"--peer-allowed-caller=spiffe://liapoldus.test/server",
			"--peer-ca-file="+filepath.Join(directory, "root.pem"),
			"--peer-cert="+filepath.Join(directory, peerCertName+".pem"), "--peer-key="+filepath.Join(directory, peerCertName+"-key.pem"),
			"--peer-carrier=tcp",
		)
		child.Dir = repositoryRoot()
		child.Env = cleanEnvironment(os.Environ())
		if err := child.Start(); err != nil {
			return nil, errors.New("start forms-db replica process")
		}
		return child, nil
	}
	stop := func(child *exec.Cmd) error {
		if child == nil || child.Process == nil {
			return nil
		}
		support.Stopped(child.Process.Signal(syscall.SIGTERM))
		done := make(chan error, 1)
		go func() { done <- child.Wait() }()
		select {
		case <-time.After(8 * time.Second):
			support.Stopped(child.Process.Kill())
			<-done
			return errors.New("stop forms-db replica timed out")
		case err := <-done:
			return err
		}
	}
	one, err := start("replica-one", restOne, peerOne, "replica-one", "replica-one-peer", "spiffe://liapoldus.test/forms/replica-one")
	if err != nil {
		return err
	}
	defer func() { support.Check(stop(one)) }()
	two, err := start("replica-two", restTwo, peerTwo, "replica-two", "replica-two-peer", "spiffe://liapoldus.test/forms/replica-two")
	if err != nil {
		return err
	}
	defer func() { support.Check(stop(two)) }()
	clientOne, err := pluginClient(contract, identities.rootPEM, identities.coreClient, restOne, revocation)
	if err != nil {
		return errors.New("create first plugin lifecycle client")
	}
	defer clientOne.CloseIdleConnections()
	clientTwo, err := pluginClient(contract, identities.rootPEM, identities.coreClient, restTwo, revocation)
	if err != nil {
		return errors.New("create second plugin lifecycle client")
	}
	defer clientTwo.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := waitHealth(ctx, clientOne); err != nil {
		return errors.New("first forms-db replica did not become healthy")
	}
	if err := waitHealth(ctx, clientTwo); err != nil {
		return errors.New("second forms-db replica did not become healthy")
	}
	firstApplied, err := clientOne.Reload(ctx, sdkmodels.Reload{Generation: currentGeneration, SHA256: firstDigest, SchemaVersion: "1"})
	if err != nil || !firstApplied.Applied {
		return errors.New("first replica failed to apply generation one")
	}
	secondApplied, err := clientTwo.Reload(ctx, sdkmodels.Reload{Generation: currentGeneration, SHA256: firstDigest, SchemaVersion: "1"})
	if err != nil || !secondApplied.Applied {
		return errors.New("second replica failed to apply generation one")
	}
	firstReadiness, err := clientOne.Readiness(ctx)
	if err != nil || !firstReadiness.Ready || firstReadiness.Generation != currentGeneration {
		return errors.New("first replica readiness did not report generation one")
	}
	secondReadiness, err := clientTwo.Readiness(ctx)
	if err != nil || !secondReadiness.Ready || secondReadiness.Generation != currentGeneration {
		return errors.New("second replica readiness did not report generation one")
	}
	callerRegistry, err := peer.NewRegistry().Build()
	if err != nil {
		return errors.New("build peer caller registry")
	}
	call := func(endpoint, peerIdentity, method string, payload []byte) response {
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(identities.rootPEM) {
			return response{}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		connection, err := peer.Dial(ctx, peer.ClientConfig{
			Network: peer.NetworkConfig{Carrier: peer.CarrierTCP, Endpoint: endpoint},
			Security: peer.SecurityConfig{Identity: "spiffe://liapoldus.test/server", Certificate: identities.caller.pair,
				Roots: roots, PeerIdentity: peerIdentity}, Handler: callerRegistry,
		})
		if err != nil {
			return response{}
		}
		defer support.Close(connection)
		result, err := connection.Call(ctx, peer.Method(method), payload)
		if err != nil {
			return response{}
		}
		var decoded struct {
			Status int    `json:"status"`
			Body   string `json:"body"`
		}
		if json.Unmarshal(result.Payload, &decoded) != nil {
			return response{}
		}
		return response{status: decoded.Status, body: decoded.Body}
	}
	for _, email := range []string{"replica-a@example.test", "replica-b@example.test"} {
		payload, fixtureErr := json.Marshal(map[string]any{"site": "portal", "schemaName": "contact", "data": map[string]string{"email": email}})
		support.Check(fixtureErr)
		if result := call(peerOne, "spiffe://liapoldus.test/forms/replica-one", "forms.submit", payload); result.status != http.StatusOK {
			return errors.New("first replica did not persist a submitted form")
		}
	}
	firstPage := call(peerOne, "spiffe://liapoldus.test/forms/replica-one", "forms.list", []byte(`{"site":"portal","schemaName":"contact","limit":1}`))
	var page struct {
		Items []json.RawMessage `json:"items"`
		Next  *string           `json:"nextCursor"`
	}
	if firstPage.status != http.StatusOK || json.Unmarshal([]byte(firstPage.body), &page) != nil || len(page.Items) != 1 || page.Next == nil {
		return errors.New("first replica did not produce a page cursor")
	}
	secondRequest, fixtureErr := json.Marshal(map[string]any{"site": "portal", "schemaName": "contact", "limit": 1, "cursor": *page.Next})
	support.Check(fixtureErr)
	continued := call(peerTwo, "spiffe://liapoldus.test/forms/replica-two", "forms.list", secondRequest)
	var secondPage struct {
		Items []json.RawMessage `json:"items"`
	}
	if continued.status != http.StatusOK || json.Unmarshal([]byte(continued.body), &secondPage) != nil || len(secondPage.Items) != 1 {
		return errors.New("second process could not continue the first process cursor")
	}
	candidateAck, err := clientOne.Reload(ctx, sdkmodels.Reload{Generation: candidateGeneration, SHA256: secondDigest, SchemaVersion: "1"})
	if err != nil || !candidateAck.Applied {
		return errors.New("first replica failed to apply candidate generation")
	}
	oneAfter, err := clientOne.Readiness(ctx)
	if err != nil || !oneAfter.Ready || oneAfter.Generation != candidateGeneration {
		return errors.New("first replica did not advance its readiness generation")
	}
	twoAfter, err := clientTwo.Readiness(ctx)
	if err != nil || !twoAfter.Ready || twoAfter.Generation != currentGeneration {
		return errors.New("second replica changed generation without reload")
	}
	invalidPayload := []byte(`{"site":"portal","schemaName":"contact","data":{"email":"old-schema@example.test"}}`)
	firstCall := call(peerOne, "spiffe://liapoldus.test/forms/replica-one", "forms.submit", invalidPayload)
	secondCall := call(peerTwo, "spiffe://liapoldus.test/forms/replica-two", "forms.submit", invalidPayload)
	if firstCall.status != http.StatusUnprocessableEntity || secondCall.status != http.StatusOK {
		return errors.New("plugin processes did not fence active schemas independently")
	}
	mixedGenerationRequest, fixtureErr := json.Marshal(map[string]any{
		"site": "portal", "schemaName": "contact", "limit": 1, "cursor": *page.Next,
	})
	support.Check(fixtureErr)
	firstMixedPage := call(peerOne, "spiffe://liapoldus.test/forms/replica-one", "forms.list", mixedGenerationRequest)
	secondMixedPage := call(peerTwo, "spiffe://liapoldus.test/forms/replica-two", "forms.list", mixedGenerationRequest)
	if firstMixedPage.status != http.StatusOK || secondMixedPage.status != http.StatusOK {
		return errors.New("cursor could not continue on both mixed-generation replicas")
	}
	for _, result := range []response{firstMixedPage, secondMixedPage} {
		var mixedPage struct {
			Items []json.RawMessage `json:"items"`
		}
		if json.Unmarshal([]byte(result.body), &mixedPage) != nil || len(mixedPage.Items) != 1 {
			return errors.New("mixed-generation cursor returned an invalid page")
		}
	}
	if err := stop(one); err != nil {
		return errors.New("stop first shared SQL replica")
	}
	one = nil
	if err := stop(two); err != nil {
		return errors.New("stop second shared SQL replica")
	}
	two = nil
	if driver != "sqlite" {
		if err := dropSharedTables(driver, databaseDSN, tablePrefix); err != nil {
			return errors.New("remove shared SQL replica test tables")
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"databaseDriver":    driver,
		"firstReplicaReady": true, "secondReplicaReady": true,
		"cursorContinuedAcrossProcesses": true, "firstReplicaAdvanced": true,
		"secondReplicaRemainedOnPreviousGeneration": true, "processGenerationFence": true,
		"cursorContinuedAcrossMixedGenerations": true,
	})
}

func dropSharedTables(driver, dsn, prefix string) error {
	sqlDriver, quote := "pgx", `"`
	if driver == "mysql" {
		sqlDriver, quote = "mysql", "`"
	}
	database, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		return errors.New("open shared SQL replica database for cleanup")
	}
	defer support.Close(database)
	for _, table := range []string{"submissions", "schemas"} {
		name := quote + prefix + table + quote
		if _, err := database.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+name); err != nil {
			return errors.New("drop shared SQL replica test table")
		}
	}
	return nil
}

type fixtureIdentities struct {
	root                   *x509.Certificate
	rootKey                *ecdsa.PrivateKey
	rootPEM                []byte
	coreServer, coreClient identity
	replicaOne, replicaTwo identity
	replicaOnePeer         identity
	replicaTwoPeer         identity
	caller                 identity
}

func issueIdentities() (fixtureIdentities, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fixtureIdentities{}, err
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "forms fixture root"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return fixtureIdentities{}, err
	}
	root, err := x509.ParseCertificate(der)
	if err != nil {
		return fixtureIdentities{}, err
	}
	issue := func(serial int64, commonName, uri string) (identity, error) {
		leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return identity{}, err
		}
		leaf := &x509.Certificate{
			SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: commonName},
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
			leaf.URIs = []*url.URL{parsed}
		}
		leafDER, err := x509.CreateCertificate(rand.Reader, leaf, root, &leafKey.PublicKey, key)
		if err != nil {
			return identity{}, err
		}
		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
		keyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
		if err != nil {
			return identity{}, err
		}
		keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
		pair, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			return identity{}, err
		}
		return identity{pair: pair, cert: certPEM, key: keyPEM}, nil
	}
	coreServer, err := issue(2, "core-server", "")
	if err != nil {
		return fixtureIdentities{}, err
	}
	coreClient, err := issue(3, "core-client", "")
	if err != nil {
		return fixtureIdentities{}, err
	}
	one, err := issue(4, "forms-replica", "")
	if err != nil {
		return fixtureIdentities{}, err
	}
	two, err := issue(5, "forms-replica", "")
	if err != nil {
		return fixtureIdentities{}, err
	}
	onePeer, err := issue(6, "forms-peer-one", "spiffe://liapoldus.test/forms/replica-one")
	if err != nil {
		return fixtureIdentities{}, err
	}
	twoPeer, err := issue(7, "forms-peer-two", "spiffe://liapoldus.test/forms/replica-two")
	if err != nil {
		return fixtureIdentities{}, err
	}
	caller, err := issue(8, "server-caller", "spiffe://liapoldus.test/server")
	if err != nil {
		return fixtureIdentities{}, err
	}
	return fixtureIdentities{
		root: root, rootKey: key, rootPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		coreServer: coreServer, coreClient: coreClient, replicaOne: one, replicaTwo: two,
		replicaOnePeer: onePeer, replicaTwoPeer: twoPeer, caller: caller,
	}, nil
}

func settings(dsnReference string, candidate bool, driver, tablePrefix string) ([]byte, error) {
	contact := map[string]any{
		"type": "object", "properties": map[string]any{"email": map[string]any{"type": "string"}},
		"required": []string{"email"}, "additionalProperties": false,
	}
	if candidate {
		contact["properties"] = map[string]any{
			"email": map[string]any{"type": "string"}, "phone": map[string]any{"type": "string"},
		}
		contact["required"] = []string{"email", "phone"}
	}
	return json.Marshal(map[string]any{
		"driver": driver, "dsn": dsnReference, "cursorSecretRef": "secret:cursor",
		"tablePrefix": tablePrefix,
		"schemas":     map[string]any{"contact": contact},
	})
}

func digest(contents []byte) string {
	hash := sha256.Sum256(contents)
	return hex.EncodeToString(hash[:])
}

func credentials(contract sdkinfra.HTTPContract, roots []byte, server, client identity) (*sdkinfra.StaticCredentialsProvider, error) {
	material, err := sdkinfra.LoadCredentials(contract, sdkinfra.CredentialsMaterial{
		CABundle: roots, ServerCertificatePEM: server.cert, ServerKeyPEM: server.key,
		ClientCertificatePEM: client.cert, ClientKeyPEM: client.key,
	})
	if err != nil {
		return nil, err
	}
	return sdkinfra.NewStaticCredentialsProvider(material)
}

func pluginClient(contract sdkinfra.HTTPContract, roots []byte, coreClient identity, endpoint string, revocation *sdkinfra.Revocation) (*sdkinfra.PluginClient, error) {
	provider, err := credentials(contract, roots, coreClient, coreClient)
	if err != nil {
		return nil, err
	}
	client, err := sdkinfra.NewMutualTLSClient(contract, provider, sdkinfra.MutualTLSClientConfig{
		Peer: sdkmodels.PeerIdentity{CommonName: "forms-replica"}, ServerName: "localhost", Revocation: revocation,
	})
	if err != nil {
		return nil, err
	}
	return sdkinfra.NewPluginClient(contract, "https://"+endpoint, client, sdkmodels.PeerIdentity{CommonName: "forms-replica"})
}

func waitHealth(ctx context.Context, client *sdkinfra.PluginClient) error {
	for ctx.Err() == nil {
		if client.Health(ctx) == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("health deadline")
}

func freeAddress() (string, error) {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	address := listener.Addr().String()
	return address, listener.Close()
}

func cleanEnvironment(environment []string) []string {
	clean := make([]string, 0, len(environment)+2)
	for _, value := range environment {
		if strings.HasPrefix(value, "FORMS_DB_") || strings.HasPrefix(value, "FORMS_CHILD_SQL_") {
			continue
		}
		clean = append(clean, value)
	}
	return append(clean, "GOWORK=off", "GOTOOLCHAIN=go1.26.0")
}

func repositoryRoot() string {
	_, current, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(current), "..", "..", "..", ".."))
}
