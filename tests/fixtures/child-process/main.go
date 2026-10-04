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
	"database/sql"
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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	productcontracts "github.com/Liapoldus/forms-db/internal/infrastructure/contracts"
	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	sdkinfra "github.com/Liapoldus/plugin-sdk/infrastructure"
	"github.com/Liapoldus/pluginprotocol/v2/presentation/peer"
	mysqlDriver "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

const generation = "forms-child-generation-1"

type identity struct {
	certificate    tls.Certificate
	certificatePEM []byte
	keyPEM         []byte
}

type adminSurfaceVector struct {
	Name            string          `json:"name"`
	ExpectedPayload json.RawMessage `json:"expectedPayload"`
}

type requestJSONVectorSet struct {
	InvalidRequests []struct {
		Name           string `json:"name"`
		Capability     string `json:"capability"`
		Payload        string `json:"payload"`
		ExpectedStatus int    `json:"expectedStatus"`
		ExpectedCode   string `json:"expectedCode"`
	} `json:"invalidRequests"`
}

type submitNegativeVectorSet struct {
	Cases []struct {
		Name           string          `json:"name"`
		Payload        json.RawMessage `json:"payload"`
		ExpectedStatus int             `json:"expectedStatus"`
		ExpectedCode   string          `json:"expectedCode"`
	} `json:"cases"`
}

type deleteNegativeVectorSet []struct {
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
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
	mu                   sync.Mutex
	next                 int
	issued               map[string]issuedGrant
	secrets              map[string]map[string][]byte
	candidateDSNRedeemed bool
	cursorIssued         int
	cursorRedeemed       int
	lastCursorHandle     string
}

type issuedGrant struct {
	value      []byte
	generation string
	reference  string
}

type synchronizedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (buffer *synchronizedBuffer) Write(value []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.Buffer.Write(value)
}

func (buffer *synchronizedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.Buffer.String()
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
	dsn := os.Getenv("FORMS_CHILD_SQL_DSN")
	if dsn == "" {
		dsn = filepath.Join(directory, "forms.sqlite")
	}
	driver := os.Getenv("FORMS_CHILD_SQL_DRIVER")
	if driver == "" {
		driver = "sqlite"
	}
	if driver != "sqlite" && driver != "mysql" && driver != "postgres" {
		return errors.New("invalid child fixture database driver")
	}
	tableSuffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	tablePrefix := "f_" + tableSuffix
	migrationFailureDSN := ""
	migrationFailureDriver := "sqlite"
	if driver != "sqlite" && os.Getenv("FORMS_CHILD_SQL_ADMIN_DSN") != "" {
		migrationFailureDSN, err = createDDLBlockedDSN(driver, os.Getenv("FORMS_CHILD_SQL_ADMIN_DSN"), tableSuffix)
		if err != nil {
			return errors.New("prepare SQL migration failure candidate")
		}
		migrationFailureDriver = driver
		defer dropDDLBlockedIdentity(driver, os.Getenv("FORMS_CHILD_SQL_ADMIN_DSN"), tableSuffix)
	}
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
	raw, err := json.Marshal(map[string]any{
		"driver": driver, "dsn": "secret:forms-child-dsn", "cursorSecretRef": "secret:forms-child-cursor",
		"tablePrefix": tablePrefix,
		"schemas":     map[string]any{"contact": map[string]any{"type": "object", "properties": map[string]any{"email": map[string]any{"type": "string"}}, "required": []string{"email"}, "additionalProperties": false}},
	})
	if err != nil {
		return errors.New("encode child fixture configuration")
	}
	digest := sha256.Sum256(raw)
	digestHex := hex.EncodeToString(digest[:])
	blockedParent := filepath.Join(directory, "not-a-directory")
	if err := os.WriteFile(blockedParent, []byte("fixture"), 0o600); err != nil {
		return err
	}
	candidateRaw, err := json.Marshal(map[string]any{
		"driver": "sqlite", "dsn": "secret:forms-child-dsn-unavailable", "cursorSecretRef": "secret:forms-child-cursor",
		"tablePrefix": "ff_" + tableSuffix,
		"schemas":     map[string]any{"contact": map[string]any{"type": "object", "properties": map[string]any{"email": map[string]any{"type": "string"}}, "required": []string{"email"}, "additionalProperties": false}},
	})
	if err != nil {
		return errors.New("encode failed child fixture candidate")
	}
	candidateDigest := sha256.Sum256(candidateRaw)
	candidateDigestHex := hex.EncodeToString(candidateDigest[:])
	candidateGeneration := "forms-child-generation-2"
	invalidSchemaGeneration := "forms-child-generation-3"
	invalidSchemaRaw, err := json.Marshal(map[string]any{
		"driver": driver, "dsn": "secret:forms-child-dsn", "cursorSecretRef": "secret:forms-child-cursor",
		"tablePrefix": "fi_" + tableSuffix,
		"schemas": map[string]any{"contact": map[string]any{
			"$ref": "https://example.invalid/unavailable-schema.json",
		}},
	})
	if err != nil {
		return errors.New("encode invalid-schema child fixture candidate")
	}
	invalidSchemaDigest := sha256.Sum256(invalidSchemaRaw)
	invalidSchemaDigestHex := hex.EncodeToString(invalidSchemaDigest[:])
	connectionFailureGeneration := "forms-child-generation-4"
	connectionFailureRaw, err := json.Marshal(map[string]any{
		"driver": driver, "dsn": "secret:forms-child-dsn-unavailable", "cursorSecretRef": "secret:forms-child-cursor",
		"tablePrefix": "fc_" + tableSuffix,
		"schemas":     map[string]any{"contact": map[string]any{"type": "object", "properties": map[string]any{"email": map[string]any{"type": "string"}}, "required": []string{"email"}, "additionalProperties": false}},
	})
	if err != nil {
		return errors.New("encode connection-failure child fixture candidate")
	}
	connectionFailureDigest := sha256.Sum256(connectionFailureRaw)
	connectionFailureDigestHex := hex.EncodeToString(connectionFailureDigest[:])
	connectionFailureDSN := unavailableDSN(driver)
	secretGrantFailureGeneration := "forms-child-generation-5"
	secretGrantFailureRaw, err := json.Marshal(map[string]any{
		"driver": driver, "dsn": "secret:forms-child-dsn-ungranted", "cursorSecretRef": "secret:forms-child-cursor",
		"tablePrefix": "fg_" + tableSuffix,
		"schemas":     map[string]any{"contact": map[string]any{"type": "object", "properties": map[string]any{"email": map[string]any{"type": "string"}}, "required": []string{"email"}, "additionalProperties": false}},
	})
	if err != nil {
		return errors.New("encode secret-grant-failure child fixture candidate")
	}
	secretGrantFailureDigest := sha256.Sum256(secretGrantFailureRaw)
	secretGrantFailureDigestHex := hex.EncodeToString(secretGrantFailureDigest[:])
	migrationFailureGeneration := "forms-child-generation-6"
	if driver == "sqlite" {
		migrationFailurePath := filepath.Join(directory, "readonly-candidate.sqlite")
		if err := os.WriteFile(migrationFailurePath, []byte{}, 0o600); err != nil {
			return errors.New("prepare read-only migration fixture")
		}
		migrationFailureDSN = "file:" + filepath.ToSlash(migrationFailurePath) + "?mode=ro"
		readonlyDB, err := sql.Open("sqlite", migrationFailureDSN)
		if err != nil {
			return errors.New("open read-only migration fixture")
		}
		if err := readonlyDB.Ping(); err != nil {
			_ = readonlyDB.Close()
			return errors.New("read-only migration fixture does not accept connections")
		}
		if _, err := readonlyDB.Exec("CREATE TABLE migration_probe (id INTEGER PRIMARY KEY)"); err == nil {
			_ = readonlyDB.Close()
			return errors.New("read-only migration fixture unexpectedly accepts DDL")
		}
		if err := readonlyDB.Close(); err != nil {
			return errors.New("close read-only migration fixture")
		}
	} else if migrationFailureDSN == "" {
		migrationFailureDSN = "secret:forms-child-dsn-readonly"
	}
	migrationFailureRaw, err := json.Marshal(map[string]any{
		"driver": migrationFailureDriver, "dsn": "secret:forms-child-dsn-readonly", "cursorSecretRef": "secret:forms-child-cursor",
		"tablePrefix": "fm_" + tableSuffix,
		"schemas":     map[string]any{"contact": map[string]any{"type": "object", "properties": map[string]any{"email": map[string]any{"type": "string"}}, "required": []string{"email"}, "additionalProperties": false}},
	})
	if err != nil {
		return errors.New("encode migration-failure child fixture candidate")
	}
	migrationFailureDigest := sha256.Sum256(migrationFailureRaw)
	migrationFailureDigestHex := hex.EncodeToString(migrationFailureDigest[:])
	grants := &grantState{
		issued: map[string]issuedGrant{},
		secrets: map[string]map[string][]byte{
			generation: {
				"secret:forms-child-dsn":    []byte(dsn),
				"secret:forms-child-cursor": bytes.Repeat([]byte("k"), 32),
			},
			candidateGeneration: {
				"secret:forms-child-dsn-unavailable": []byte(filepath.Join(blockedParent, "forms.sqlite")),
				"secret:forms-child-cursor":          bytes.Repeat([]byte("k"), 32),
			},
			invalidSchemaGeneration: {
				"secret:forms-child-dsn":    []byte(dsn),
				"secret:forms-child-cursor": bytes.Repeat([]byte("k"), 32),
			},
			connectionFailureGeneration: {
				"secret:forms-child-dsn-unavailable": connectionFailureDSN,
				"secret:forms-child-cursor":          bytes.Repeat([]byte("k"), 32),
			},
			secretGrantFailureGeneration: {
				"secret:forms-child-cursor": bytes.Repeat([]byte("k"), 32),
			},
			migrationFailureGeneration: {
				"secret:forms-child-dsn-readonly": []byte(migrationFailureDSN),
				"secret:forms-child-cursor":       bytes.Repeat([]byte("k"), 32),
			},
		},
	}
	coreMux := http.NewServeMux()
	coreMux.HandleFunc(contract.Core.ConfigPull.Method+" "+contract.Core.ConfigPull.PathTemplate, func(writer http.ResponseWriter, request *http.Request) {
		documentGeneration := request.PathValue("generation")
		document, ok := map[string]struct {
			raw    []byte
			digest string
		}{
			generation:          {raw: raw, digest: digestHex},
			candidateGeneration: {raw: candidateRaw, digest: candidateDigestHex},
			invalidSchemaGeneration: {
				raw: invalidSchemaRaw, digest: invalidSchemaDigestHex,
			},
			connectionFailureGeneration: {
				raw: connectionFailureRaw, digest: connectionFailureDigestHex,
			},
			secretGrantFailureGeneration: {
				raw: secretGrantFailureRaw, digest: secretGrantFailureDigestHex,
			},
			migrationFailureGeneration: {
				raw: migrationFailureRaw, digest: migrationFailureDigestHex,
			},
		}[documentGeneration]
		if !ok {
			http.NotFound(writer, request)
			return
		}
		headers := contract.Core.ConfigPull.ResponseHeaders
		writer.Header().Set("content-type", contract.Core.ConfigPull.ResponseMediaType)
		writer.Header().Set(headers["generation"], documentGeneration)
		writer.Header().Set(headers["schemaVersion"], "1")
		writer.Header().Set(headers["sha256"], document.digest)
		writer.Header().Set(headers["generationState"], "active")
		_, _ = writer.Write(document.raw)
	})
	coreMux.HandleFunc(contract.Core.SecretGrant.Issue.Method+" "+contract.Core.SecretGrant.Issue.PathTemplate, func(writer http.ResponseWriter, request *http.Request) {
		var query sdkmodels.SecretGrantRequest
		if json.NewDecoder(io.LimitReader(request.Body, 4096)).Decode(&query) != nil || query.Purpose == "" {
			http.Error(writer, "grant refused", http.StatusForbidden)
			return
		}
		grants.mu.Lock()
		secret, allowed := grants.secrets[query.Generation][query.Reference]
		if !allowed {
			grants.mu.Unlock()
			http.Error(writer, "grant refused", http.StatusForbidden)
			return
		}
		grants.next++
		handle := fmt.Sprintf("forms-grant-%d", grants.next)
		grants.issued[handle] = issuedGrant{value: secret, generation: query.Generation, reference: query.Reference}
		if query.Reference == "secret:forms-child-cursor" {
			grants.cursorIssued++
			grants.lastCursorHandle = handle
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
		grant, allowed := grants.issued[handle]
		delete(grants.issued, handle)
		if allowed && grant.reference == "secret:forms-child-cursor" {
			grants.cursorRedeemed++
		}
		if allowed && grant.generation == candidateGeneration && grant.reference == "secret:forms-child-dsn-unavailable" {
			grants.candidateDSNRedeemed = true
		}
		grants.mu.Unlock()
		if !allowed {
			http.Error(writer, "grant spent", http.StatusForbidden)
			return
		}
		writer.Header().Set("content-type", contract.Core.SecretGrant.Redemption.ResponseMediaType)
		_, _ = writer.Write(grant.value)
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
	var processOutput synchronizedBuffer
	start := func() (*exec.Cmd, error) {
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
		child.Env = make([]string, 0, len(os.Environ()))
		for _, item := range os.Environ() {
			if strings.HasPrefix(item, "FORMS_DB_") || strings.HasPrefix(item, "FORMS_CHILD_SQL_") {
				continue
			}
			child.Env = append(child.Env, item)
		}
		child.Dir = repositoryRoot()
		child.Stdout, child.Stderr = &processOutput, &processOutput
		return child, child.Start()
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
		child, err := start()
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
	status, body, err := call("forms.submit", []byte(`{"site":"`+tableSuffix+`","schemaName":"contact","data":{"email":"persist@example.test"}}`))
	if err != nil || status != 200 {
		return fmt.Errorf("submit failed: %v / %d", err, status)
	}
	var submitted struct {
		ID string `json:"id"`
	}
	if json.Unmarshal([]byte(body), &submitted) != nil || submitted.ID == "" {
		return errors.New("submit returned no ID")
	}
	var requestVectors requestJSONVectorSet
	if json.Unmarshal([]byte(os.Getenv("FORMS_REQUEST_JSON_VECTORS")), &requestVectors) != nil || len(requestVectors.InvalidRequests) == 0 {
		return errors.New("request JSON vectors unavailable")
	}
	productRequestVectorsRefused := true
	for _, vector := range requestVectors.InvalidRequests {
		if vector.Name == "" || vector.ExpectedCode == "" || vector.ExpectedStatus == 0 {
			productRequestVectorsRefused = false
			continue
		}
		vectorStatus, vectorBody, vectorErr := call(vector.Capability, []byte(vector.Payload))
		var errorBody struct {
			Code string `json:"code"`
		}
		if vectorErr != nil || vectorStatus != vector.ExpectedStatus || json.Unmarshal([]byte(vectorBody), &errorBody) != nil || errorBody.Code != vector.ExpectedCode {
			productRequestVectorsRefused = false
		}
	}
	var submitVectors submitNegativeVectorSet
	if json.Unmarshal([]byte(os.Getenv("FORMS_SUBMIT_NEGATIVE_VECTORS")), &submitVectors) != nil || len(submitVectors.Cases) == 0 {
		return errors.New("submit negative vectors unavailable")
	}
	for _, vector := range submitVectors.Cases {
		if vector.Name == "" || len(vector.Payload) == 0 || vector.ExpectedCode == "" || vector.ExpectedStatus == 0 {
			productRequestVectorsRefused = false
			continue
		}
		vectorStatus, vectorBody, vectorErr := call("forms.submit", vector.Payload)
		var errorBody struct {
			Code string `json:"code"`
		}
		if vectorErr != nil || vectorStatus != vector.ExpectedStatus || json.Unmarshal([]byte(vectorBody), &errorBody) != nil || errorBody.Code != vector.ExpectedCode {
			productRequestVectorsRefused = false
		}
	}
	var deleteVectors deleteNegativeVectorSet
	if json.Unmarshal([]byte(os.Getenv("FORMS_DELETE_NEGATIVE_VECTORS")), &deleteVectors) != nil || len(deleteVectors) == 0 {
		return errors.New("delete negative vectors unavailable")
	}
	for _, vector := range deleteVectors {
		if vector.Name == "" || len(vector.Payload) == 0 {
			productRequestVectorsRefused = false
			continue
		}
		vectorStatus, vectorBody, vectorErr := call("forms.delete", vector.Payload)
		var errorBody struct {
			Code string `json:"code"`
		}
		if vectorErr != nil || vectorStatus != http.StatusUnprocessableEntity || json.Unmarshal([]byte(vectorBody), &errorBody) != nil || errorBody.Code != "validation_failed" {
			productRequestVectorsRefused = false
		}
	}
	candidateAck, candidateErr := pluginClient.Reload(context.Background(), sdkmodels.Reload{
		Generation: candidateGeneration, SHA256: candidateDigestHex, SchemaVersion: "1",
	})
	grants.mu.Lock()
	candidateDSNRedeemed := grants.candidateDSNRedeemed
	grants.mu.Unlock()
	if candidateErr == nil || candidateAck.Applied || !candidateDSNRedeemed {
		return fmt.Errorf("candidate storage-open failure returned applied=%t outcome=%s errorType=%T", candidateAck.Applied, candidateAck.Outcome, candidateErr)
	}
	candidateBuildFailurePreserved := candidateDSNRedeemed
	site := tableSuffix
	status, body, err = call("forms.list", []byte(`{"site":"`+site+`","schemaName":"contact","limit":10}`))
	if err != nil || status != 200 {
		return errors.New("active forms repository stopped serving after candidate failure")
	}
	var afterCandidate struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if json.Unmarshal([]byte(body), &afterCandidate) != nil || len(afterCandidate.Items) != 1 || afterCandidate.Items[0].ID != submitted.ID {
		return errors.New("active forms repository lost its previous data after candidate failure")
	}
	grants.mu.Lock()
	firstCursorIssued, firstCursorRedeemed := grants.cursorIssued, grants.cursorRedeemed
	spentCursorHandle := grants.lastCursorHandle
	_, handleStillAvailable := grants.issued[spentCursorHandle]
	grants.mu.Unlock()
	if firstCursorIssued != 1 || firstCursorRedeemed != 1 || spentCursorHandle == "" || handleStillAvailable {
		return errors.New("forms list did not consume one cursor grant")
	}
	coreURL, err := url.Parse("https://" + coreListener.Addr().String())
	if err != nil {
		return err
	}
	replayURL, err := contract.ControlURL(coreURL, contract.Core.SecretGrant.Redemption.PathTemplate, spentCursorHandle, "handle")
	if err != nil {
		return err
	}
	replayDocument, err := json.Marshal(sdkmodels.SecretRedemption{Handle: spentCursorHandle})
	if err != nil {
		return err
	}
	replayRequest, err := http.NewRequest(http.MethodPost, replayURL.String(), bytes.NewReader(replayDocument))
	if err != nil {
		return err
	}
	trustRoots := x509.NewCertPool()
	trustRoots.AddCert(issued.root)
	replayClient := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS12, RootCAs: trustRoots, ServerName: "localhost",
		Certificates: []tls.Certificate{issued.plugin.certificate},
	}}}
	replayResponse, err := replayClient.Do(replayRequest)
	if err != nil {
		return err
	}
	_ = replayResponse.Body.Close()
	spentGrantReplayRejected := replayResponse.StatusCode == http.StatusForbidden
	if !spentGrantReplayRejected {
		return errors.New("spent cursor grant was redeemable again")
	}
	previousGenerationStillServes := true
	invalidSchemaAck, invalidSchemaErr := pluginClient.Reload(context.Background(), sdkmodels.Reload{
		Generation: invalidSchemaGeneration, SHA256: invalidSchemaDigestHex, SchemaVersion: "1",
	})
	status, body, err = call("forms.list", []byte(`{"site":"`+site+`","schemaName":"contact","limit":10}`))
	if invalidSchemaErr == nil || invalidSchemaAck.Applied || err != nil || status != http.StatusOK {
		return errors.New("invalid schema candidate changed the active serving generation")
	}
	var afterInvalidSchema struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if json.Unmarshal([]byte(body), &afterInvalidSchema) != nil || len(afterInvalidSchema.Items) != 1 || afterInvalidSchema.Items[0].ID != submitted.ID {
		return errors.New("active data changed after invalid schema candidate")
	}
	grants.mu.Lock()
	cursorGrantsPerCall := grants.cursorIssued == firstCursorIssued+1 && grants.cursorRedeemed == firstCursorRedeemed+1 &&
		grants.lastCursorHandle != spentCursorHandle
	grants.mu.Unlock()
	if !cursorGrantsPerCall {
		return errors.New("forms list reused a cursor grant across calls")
	}
	candidateSchemaFailurePreserved := true
	secretGrantAck, secretGrantErr := pluginClient.Reload(context.Background(), sdkmodels.Reload{
		Generation: secretGrantFailureGeneration, SHA256: secretGrantFailureDigestHex, SchemaVersion: "1",
	})
	status, body, err = call("forms.list", []byte(`{"site":"`+site+`","schemaName":"contact","limit":10}`))
	if secretGrantErr == nil || secretGrantAck.Applied || err != nil || status != http.StatusOK {
		return errors.New("secret grant failure candidate changed the active serving generation")
	}
	var afterSecretGrantFailure struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if json.Unmarshal([]byte(body), &afterSecretGrantFailure) != nil || len(afterSecretGrantFailure.Items) != 1 || afterSecretGrantFailure.Items[0].ID != submitted.ID {
		return errors.New("active data changed after secret grant failure")
	}
	candidateSecretGrantFailurePreserved := true
	candidateMigrationFailurePreserved := false
	if driver == "sqlite" || migrationFailureDriver == driver {
		migrationAck, migrationErr := pluginClient.Reload(context.Background(), sdkmodels.Reload{
			Generation: migrationFailureGeneration, SHA256: migrationFailureDigestHex, SchemaVersion: "1",
		})
		status, body, err = call("forms.list", []byte(`{"site":"`+site+`","schemaName":"contact","limit":10}`))
		if migrationErr == nil || migrationAck.Applied || err != nil || status != http.StatusOK {
			return errors.New("SQLite DDL failure candidate changed the active serving generation")
		}
		var afterMigrationFailure struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		if json.Unmarshal([]byte(body), &afterMigrationFailure) != nil || len(afterMigrationFailure.Items) != 1 || afterMigrationFailure.Items[0].ID != submitted.ID {
			return errors.New("active data changed after SQLite DDL failure")
		}
		candidateMigrationFailurePreserved = true
	}
	candidateConnectionFailurePreserved := false
	if driver != "sqlite" {
		connectionAck, connectionErr := pluginClient.Reload(context.Background(), sdkmodels.Reload{
			Generation: connectionFailureGeneration, SHA256: connectionFailureDigestHex, SchemaVersion: "1",
		})
		status, body, err = call("forms.list", []byte(`{"site":"`+site+`","schemaName":"contact","limit":10}`))
		if connectionErr == nil || connectionAck.Applied || err != nil || status != http.StatusOK {
			return errors.New("SQL connection failure candidate changed the active serving generation")
		}
		var afterConnectionFailure struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		if json.Unmarshal([]byte(body), &afterConnectionFailure) != nil || len(afterConnectionFailure.Items) != 1 || afterConnectionFailure.Items[0].ID != submitted.ID {
			return errors.New("active data changed after SQL connection failure")
		}
		candidateConnectionFailurePreserved = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	surface, surfaceErr := pluginClient.AdminSurface(ctx)
	if surfaceErr != nil {
		cancel()
		return fmt.Errorf("admin surface: %w", surfaceErr)
	}
	expectedSurface, contractErr := productcontracts.AdminSurface()
	if contractErr != nil || !bytes.Equal(surface.Bytes, expectedSurface) || surface.SHA256 != "sha256:"+sdkmodels.Digest(expectedSurface) {
		cancel()
		return errors.New("SDK admin surface differs from the exact plugin-owned contract")
	}
	var adminSurfaceVectors []adminSurfaceVector
	if json.Unmarshal([]byte(os.Getenv("FORMS_ADMIN_SURFACE_VECTORS")), &adminSurfaceVectors) != nil || len(adminSurfaceVectors) == 0 {
		cancel()
		return errors.New("admin surface vectors unavailable")
	}
	adminSurfaceVectorRuntimeAccepted := true
	for _, vector := range adminSurfaceVectors {
		if vector.Name == "" || len(vector.ExpectedPayload) == 0 {
			adminSurfaceVectorRuntimeAccepted = false
			continue
		}
		vectorResult, vectorErr := pluginClient.AdminAction(ctx, sdkmodels.AdminActionInvocation{
			CallerID: "operator-test", InstanceID: "forms-child", PageID: "submissions",
			ActionID: "delete", SurfaceDigest: surface.SHA256, RequestID: "vector-" + vector.Name,
		}, vector.ExpectedPayload)
		if vectorErr != nil || vectorResult.StatusCode != http.StatusNotFound {
			adminSurfaceVectorRuntimeAccepted = false
		}
	}
	adminList, listErr := pluginClient.AdminAction(ctx, sdkmodels.AdminActionInvocation{
		CallerID: "operator-test", InstanceID: "forms-child", PageID: "submissions",
		ActionID: "query", SurfaceDigest: surface.SHA256, RequestID: "request-admin-list",
	}, []byte(`{"site":"`+site+`","schemaName":"contact","limit":10}`))
	cancel()
	if listErr != nil || adminList.StatusCode != http.StatusOK {
		return fmt.Errorf("SDK admin query failed: %v / %d", listErr, adminList.StatusCode)
	}
	var adminListBody struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if json.Unmarshal(adminList.Body, &adminListBody) != nil || len(adminListBody.Items) != 1 || adminListBody.Items[0].ID != submitted.ID {
		return errors.New("SDK admin query did not expose the submitted record")
	}
	if err := stop(child); err != nil {
		return err
	}
	child = nil
	child, err = reload()
	if err != nil {
		return err
	}
	status, body, err = call("forms.list", []byte(`{"site":"`+site+`","schemaName":"contact","limit":10}`))
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
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	adminDelete, deleteErr := pluginClient.AdminAction(ctx, sdkmodels.AdminActionInvocation{
		CallerID: "operator-test", InstanceID: "forms-child", PageID: "submissions",
		ActionID: "delete", SurfaceDigest: surface.SHA256, RequestID: "request-admin-delete",
		IdempotencyKey: "delete-" + submitted.ID,
	}, []byte(`{"site":"`+site+`","schemaName":"contact","id":"`+submitted.ID+`"}`))
	cancel()
	if deleteErr != nil || adminDelete.StatusCode != http.StatusOK {
		return fmt.Errorf("SDK admin delete failed: %v / %d", deleteErr, adminDelete.StatusCode)
	}
	var deleted struct {
		Deleted bool   `json:"deleted"`
		ID      string `json:"id"`
	}
	if json.Unmarshal(adminDelete.Body, &deleted) != nil || !deleted.Deleted || deleted.ID != submitted.ID {
		return errors.New("SDK admin delete returned an invalid receipt")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	missingDelete, missingDeleteErr := pluginClient.AdminAction(ctx, sdkmodels.AdminActionInvocation{
		CallerID: "operator-test", InstanceID: "forms-child", PageID: "submissions",
		ActionID: "delete", SurfaceDigest: surface.SHA256, RequestID: "request-admin-delete-missing",
	}, []byte(`{"site":"`+site+`","schemaName":"contact","id":"`+submitted.ID+`"}`))
	cancel()
	if missingDeleteErr != nil || missingDelete.StatusCode != http.StatusNotFound {
		return fmt.Errorf("repeated SDK admin delete must be not-found: %v / %d", missingDeleteErr, missingDelete.StatusCode)
	}
	serverToForms := false
	if os.Getenv("FORMS_CHILD_SKIP_SERVER") != "1" {
		if err := runServerToForms(directory, issued, peerAddress, contract); err != nil {
			return err
		}
		serverToForms = true
	}
	_, _, wrongCapabilityErr := call("forms.unknown", []byte(`{"site":"wrong-capability-check","schemaName":"contact","data":{"email":"must-not-be-stored@example.test"}}`))
	wrongCapabilityRejected := errors.Is(wrongCapabilityErr, peer.ErrMethodNotFound)
	if !wrongCapabilityRejected {
		return errors.New("unregistered product capability was not refused")
	}
	status, body, err = call("forms.list", []byte(`{"site":"wrong-capability-check","schemaName":"contact","limit":10}`))
	if err != nil || status != http.StatusOK {
		return errors.New("authorized query failed after unregistered capability")
	}
	var afterWrongCapability struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if json.Unmarshal([]byte(body), &afterWrongCapability) != nil || len(afterWrongCapability.Items) != 0 {
		return errors.New("unregistered capability changed the stored records")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
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
	_, err = outsider.Call(ctx, peer.Method("forms.submit"), []byte(`{"site":"unauthorized-check","schemaName":"contact","data":{"email":"not-saved@example.test"}}`))
	if !errors.Is(err, peer.ErrUnauthorized) {
		return errors.New("unauthorized caller was not rejected")
	}
	_, unauthorizedListErr := outsider.Call(ctx, peer.Method("forms.list"), []byte(`{"site":"portal","schemaName":"contact","limit":10}`))
	_, unauthorizedDeleteErr := outsider.Call(ctx, peer.Method("forms.delete"), []byte(`{"site":"portal","schemaName":"contact","id":"`+submitted.ID+`"}`))
	unauthorizedReadDeleteRejected := errors.Is(unauthorizedListErr, peer.ErrUnauthorized) &&
		errors.Is(unauthorizedDeleteErr, peer.ErrUnauthorized)
	if !unauthorizedReadDeleteRejected {
		return errors.New("unauthorized caller was not rejected for forms list and delete")
	}
	unauthorizedError := err.Error()
	untrustedPeer, err := createUntrustedPeerIdentity()
	if err != nil {
		return err
	}
	untrustedRoots := x509.NewCertPool()
	untrustedRoots.AddCert(issued.root)
	untrustedPeerClient, err := peer.Dial(ctx, peer.ClientConfig{
		Network: peer.NetworkConfig{Carrier: peer.CarrierTCP, Endpoint: peerAddress},
		Security: peer.SecurityConfig{Identity: "spiffe://liapoldus.test/untrusted", Certificate: untrustedPeer.certificate,
			Roots: untrustedRoots, PeerIdentity: "spiffe://liapoldus.test/forms"}, Handler: callerRegistry,
	})
	untrustedPeerCARejected := err != nil
	if untrustedPeerClient != nil {
		_, callErr := untrustedPeerClient.Call(ctx, peer.Method("forms.submit"), []byte(`{"site":"untrusted-ca-check","schemaName":"contact","data":{"email":"untrusted-ca@example.test"}}`))
		untrustedPeerCARejected = callErr != nil
		_ = untrustedPeerClient.Close()
	}
	if !untrustedPeerCARejected {
		return errors.New("peer accepted client certificate from an untrusted CA")
	}
	status, body, err = call("forms.list", []byte(`{"site":"untrusted-ca-check","schemaName":"contact","limit":10}`))
	if err != nil || status != http.StatusOK {
		return errors.New("authorized verification query failed after untrusted CA rejection")
	}
	var afterUntrustedCA struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if json.Unmarshal([]byte(body), &afterUntrustedCA) != nil || len(afterUntrustedCA.Items) != 0 {
		return errors.New("untrusted peer submission reached forms storage")
	}
	status, body, err = call("forms.list", []byte(`{"site":"unauthorized-check","schemaName":"contact","limit":10}`))
	if err != nil || status != http.StatusOK {
		return errors.New("authorized verification query failed after denied caller")
	}
	var afterUnauthorized struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if json.Unmarshal([]byte(body), &afterUnauthorized) != nil || len(afterUnauthorized.Items) != 0 {
		return errors.New("unauthorized submission changed the stored records")
	}
	metricsEndpoint, endpointErr := contract.Endpoint("metrics")
	if endpointErr != nil {
		return errors.New("metrics endpoint missing from SDK contract")
	}
	metricsRequest, err := http.NewRequestWithContext(ctx, metricsEndpoint.Method, "https://"+restAddress+metricsEndpoint.Path, nil)
	if err != nil {
		return errors.New("create metrics request")
	}
	metricsResponse, err := pluginHTTP.Do(metricsRequest)
	if err != nil {
		return errors.New("read plugin metrics")
	}
	metricsBody, readErr := io.ReadAll(io.LimitReader(metricsResponse.Body, 1<<20))
	_ = metricsResponse.Body.Close()
	if readErr != nil || metricsResponse.StatusCode != http.StatusOK {
		return errors.New("plugin metrics request failed")
	}
	if err := stop(child); err != nil {
		return err
	}
	child = nil
	processOutputText := processOutput.String()
	observabilityText := processOutputText + unauthorizedError + string(metricsBody)
	noSensitiveOutput := true
	for _, marker := range []string{
		"persist@example.test", "not-saved@example.test", "secret:forms-child-dsn",
		"secret:forms-child-cursor", dsn,
	} {
		if marker != "" && strings.Contains(observabilityText, marker) {
			noSensitiveOutput = false
			break
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"reload": true, "peerSubmit": true, "sqliteRestart": true, "serverToForms": serverToForms,
		"databaseDriver": driver, "databaseRestartPersistence": true,
		"candidateBuildFailurePreserved":       candidateBuildFailurePreserved,
		"candidateSchemaFailurePreserved":      candidateSchemaFailurePreserved,
		"candidateSecretGrantFailurePreserved": candidateSecretGrantFailurePreserved,
		"candidateMigrationFailurePreserved":   candidateMigrationFailurePreserved,
		"candidateConnectionFailurePreserved":  candidateConnectionFailurePreserved,
		"cursorGrantsPerCall":                  cursorGrantsPerCall,
		"spentGrantReplayRejected":             spentGrantReplayRejected,
		"previousGenerationStillServes":        previousGenerationStillServes,
		"wrongCapabilityRejected":              wrongCapabilityRejected,
		"unauthorizedCallerRejected":           true,
		"unauthorizedReadDeleteRejected":       unauthorizedReadDeleteRejected,
		"productRequestVectorsRefused":          productRequestVectorsRefused,
		"unauthorizedPayloadNotPersisted":      true,
		"untrustedPeerCARejected": untrustedPeerCARejected,
		"adminSurface": true, "adminSurfaceVectorRuntimeAccepted": adminSurfaceVectorRuntimeAccepted,
		"adminList": true,
		"adminDelete": true, "adminDeleteMissing": true,
		"noSensitiveOutput": noSensitiveOutput,
	})
}

func createUntrustedPeerIdentity() (identity, error) {
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return identity{}, err
	}
	now := time.Now()
	rootTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(100), Subject: pkix.Name{CommonName: "untrusted peer root"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		return identity{}, err
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		return identity{}, err
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return identity{}, err
	}
	uri, err := url.Parse("spiffe://liapoldus.test/untrusted")
	if err != nil {
		return identity{}, err
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(101), Subject: pkix.Name{CommonName: "untrusted peer"},
		URIs: []*url.URL{uri}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, root, &leafKey.PublicKey, rootKey)
	if err != nil {
		return identity{}, err
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		return identity{}, err
	}
	pair := tls.Certificate{Certificate: [][]byte{leafDER, rootDER}, PrivateKey: leafKey, Leaf: leaf}
	return identity{certificate: pair}, nil
}

func unavailableDSN(driver string) []byte {
	switch driver {
	case "postgres":
		return []byte("postgres://liapoldus_test:codex_test_only@127.0.0.1:1/forms_test?sslmode=disable&connect_timeout=1")
	case "mysql":
		return []byte("liapoldus_test:codex_test_only@tcp(127.0.0.1:1)/forms_test?timeout=1s")
	default:
		return nil
	}
}

func createDDLBlockedDSN(driver, adminDSN, suffix string) (string, error) {
	username := "forms_ddl_" + suffix
	password := "denied_" + suffix
	adminDriver := "mysql"
	if driver == "postgres" {
		adminDriver = "pgx"
	}
	admin, err := sql.Open(adminDriver, adminDSN)
	if err != nil {
		return "", errors.New("open SQL admin connection")
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := admin.PingContext(ctx); err != nil {
		return "", errors.New("ping SQL admin connection")
	}

	var candidateDSN string
	if driver == "postgres" {
		adminURL, err := url.Parse(adminDSN)
		if err != nil || (adminURL.Scheme != "postgres" && adminURL.Scheme != "postgresql") {
			return "", errors.New("parse PostgreSQL admin DSN")
		}
		databaseName, err := url.PathUnescape(strings.TrimPrefix(adminURL.EscapedPath(), "/"))
		if err != nil || databaseName == "" {
			return "", errors.New("read PostgreSQL database name")
		}
		schemaName := "forms_ddl_" + suffix
		if _, err := admin.ExecContext(ctx, "CREATE ROLE \""+username+"\" LOGIN PASSWORD '"+password+"'"); err != nil {
			return "", errors.New("create PostgreSQL restricted candidate role")
		}
		if _, err := admin.ExecContext(ctx, "CREATE SCHEMA \""+schemaName+"\""); err != nil {
			dropDDLBlockedIdentity(driver, adminDSN, suffix)
			return "", errors.New("create PostgreSQL restricted candidate schema")
		}
		if _, err := admin.ExecContext(ctx, "GRANT CONNECT ON DATABASE \""+databaseName+"\" TO \""+username+"\""); err != nil {
			dropDDLBlockedIdentity(driver, adminDSN, suffix)
			return "", errors.New("grant PostgreSQL candidate connection")
		}
		if _, err := admin.ExecContext(ctx, "GRANT USAGE ON SCHEMA \""+schemaName+"\" TO \""+username+"\""); err != nil {
			dropDDLBlockedIdentity(driver, adminDSN, suffix)
			return "", errors.New("grant PostgreSQL candidate schema usage")
		}
		candidateURL := *adminURL
		candidateURL.User = url.UserPassword(username, password)
		query := candidateURL.Query()
		query.Set("search_path", schemaName)
		candidateURL.RawQuery = query.Encode()
		candidateDSN = candidateURL.String()
	} else {
		adminConfig, err := mysqlDriver.ParseDSN(adminDSN)
		if err != nil || adminConfig.DBName == "" {
			return "", errors.New("parse MySQL admin DSN")
		}
		if _, err := admin.ExecContext(ctx, "CREATE USER '"+username+"'@'%' IDENTIFIED BY '"+password+"'"); err != nil {
			return "", errors.New("create MySQL restricted candidate user")
		}
		databaseName := strings.ReplaceAll(adminConfig.DBName, "`", "``")
		if _, err := admin.ExecContext(ctx, "GRANT SELECT ON `"+databaseName+"`.* TO '"+username+"'@'%' "); err != nil {
			dropDDLBlockedIdentity(driver, adminDSN, suffix)
			return "", errors.New("grant MySQL candidate database access")
		}
		candidateConfig := *adminConfig
		candidateConfig.User = username
		candidateConfig.Passwd = password
		candidateDSN = candidateConfig.FormatDSN()
	}

	candidate, err := sql.Open(adminDriver, candidateDSN)
	if err != nil {
		dropDDLBlockedIdentity(driver, adminDSN, suffix)
		return "", errors.New("open restricted SQL candidate connection")
	}
	defer candidate.Close()
	if err := candidate.PingContext(ctx); err != nil {
		dropDDLBlockedIdentity(driver, adminDSN, suffix)
		return "", errors.New("restricted SQL candidate cannot connect")
	}
	probe := "migration_probe_" + suffix
	statement := "CREATE TABLE \"" + probe + "\" (id TEXT PRIMARY KEY)"
	if driver == "mysql" {
		statement = "CREATE TABLE `" + probe + "` (id VARCHAR(32) PRIMARY KEY)"
	}
	if _, err := candidate.ExecContext(ctx, statement); err == nil {
		dropDDLBlockedIdentity(driver, adminDSN, suffix)
		return "", errors.New("restricted SQL candidate unexpectedly accepts DDL")
	}
	return candidateDSN, nil
}

func dropDDLBlockedIdentity(driver, adminDSN, suffix string) {
	adminDriver := "mysql"
	if driver == "postgres" {
		adminDriver = "pgx"
	}
	admin, err := sql.Open(adminDriver, adminDSN)
	if err != nil {
		return
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	username := "forms_ddl_" + suffix
	if driver == "postgres" {
		_, _ = admin.ExecContext(ctx, "DROP OWNED BY \""+username+"\"")
		_, _ = admin.ExecContext(ctx, "DROP SCHEMA IF EXISTS \""+username+"\" CASCADE")
		_, _ = admin.ExecContext(ctx, "DROP ROLE IF EXISTS \""+username+"\"")
		return
	}
	_, _ = admin.ExecContext(ctx, "DROP USER IF EXISTS '"+username+"'@'%' ")
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
