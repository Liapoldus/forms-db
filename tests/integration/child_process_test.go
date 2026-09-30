package integration

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Liapoldus/forms-db/internal/infrastructure/security"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
	"github.com/Liapoldus/pluginprotocol/presentation/sdk"
)

const (
	formsSmokeInstanceID = "forms-db-child-smoke"
	formsSmokeGatewayID  = "urn:liapoldus:gateway:forms-db-child-smoke"
	formsSmokePluginID   = "urn:liapoldus:plugin:forms-db:replica:child-smoke"
	formsSmokeSecretRef  = "forms-db-child-smoke-dsn"
	formsSmokeSecretKey  = "forms-db-child-smoke-config-grant"
	formsSmokeCursorKey  = "forms-db-child-smoke-cursor-grant"
)

type formsSmokeGrantBroker struct {
	pluginv1.UnimplementedGrantBrokerServer
	dsn       []byte
	cursorKey []byte
}

func (broker *formsSmokeGrantBroker) RedeemGrant(_ context.Context, request *pluginv1.RedeemGrantRequest) (*pluginv1.RedeemGrantResponse, error) {
	if request.GetScope() == pluginv1.GrantScope_GRANT_SCOPE_CONFIG_APPLY &&
		request.GetHandle() == formsSmokeSecretKey && request.GetInstanceId() == formsSmokeInstanceID &&
		request.GetSettingsRevision() == "settings-r1" && request.GetSecretReference() == formsSmokeSecretRef &&
		request.GetPurpose() == "storage-dsn" && request.GetCapability() == "" && request.GetDomain() == "" {
		return &pluginv1.RedeemGrantResponse{Secret: append([]byte(nil), broker.dsn...)}, nil
	}
	capability, purpose, domain, ok := security.CursorGrantScope()
	if ok && request.GetScope() == pluginv1.GrantScope_GRANT_SCOPE_CALL &&
		request.GetHandle() == formsSmokeCursorKey && request.GetCapability() == capability &&
		request.GetPurpose() == purpose && request.GetDomain() == domain {
		return &pluginv1.RedeemGrantResponse{Secret: append([]byte(nil), broker.cursorKey...)}, nil
	}
	return nil, sdk.ErrGrantDenied
}

func TestFormsDBLocalChildProcessLifecycleAndSQLitePersistence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	moduleRoot := formsDBModuleRoot(t)
	artifactDirectory := t.TempDir()
	binaryPath := filepath.Join(artifactDirectory, "forms-db")
	build := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, "./cmd/forms-db")
	build.Dir = moduleRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build forms-db child process: %v\n%s", err, output)
	}
	releaseDigest := digestBytes(t, mustReadFile(t, binaryPath))

	databasePath := filepath.Join(t.TempDir(), "forms.sqlite")
	brokerService := &formsSmokeGrantBroker{
		dsn:       []byte(databasePath),
		cursorKey: []byte(strings.Repeat("k", 32)),
	}
	grantBroker, err := sdk.StartGrantBroker(brokerService)
	if err != nil {
		t.Fatalf("start private test GrantBroker: %v", err)
	}
	t.Cleanup(grantBroker.Stop)

	settings := []byte(`{"driver":"sqlite","dsn":"` + formsSmokeSecretRef + `","tablePrefix":"formsdb_smoke_","schemas":{"contact":{"type":"object","properties":{"email":{"type":"string"}},"required":["email"],"additionalProperties":false}}}`)
	settingsDigest := digestBytes(t, settings)
	configGrant := &pluginv1.ActiveGrant{
		Handle: formsSmokeSecretKey, Purpose: "storage-dsn",
		Scope: pluginv1.GrantScope_GRANT_SCOPE_CONFIG_APPLY, InstanceId: formsSmokeInstanceID,
		SettingsRevision: "settings-r1", SecretReference: formsSmokeSecretRef,
	}
	cursorCapability, cursorPurpose, cursorDomain, ok := security.CursorGrantScope()
	if !ok {
		t.Fatal("cursor grant contract is unavailable")
	}
	cursorGrant := &pluginv1.ActiveGrant{
		Handle: formsSmokeCursorKey, Purpose: cursorPurpose, Domains: []string{cursorDomain},
		Capability: cursorCapability, Scope: pluginv1.GrantScope_GRANT_SCOPE_CALL,
	}

	startSession := func() (*sdk.LocalSession, sdk.Handshake) {
		t.Helper()
		session, err := sdk.StartLocalSession(ctx, sdk.LocalSessionOptions{
			Binary: binaryPath, GatewayIdentity: formsSmokeGatewayID,
			PluginIdentity: formsSmokePluginID, ReleaseDigest: releaseDigest,
		})
		if err != nil {
			t.Fatalf("launch forms-db with inherited local identity: %v", err)
		}
		t.Cleanup(func() {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer stopCancel()
			if err := session.Stop(stopCtx); err != nil {
				t.Errorf("stop forms-db child process during cleanup: %v", err)
			}
		})
		bootstrap := &pluginv1.BootstrapRequest{InstanceId: formsSmokeInstanceID, GrantBrokerEndpoint: grantBroker.Endpoint()}
		handshake, err := session.Client().BootstrapAndHandshake(ctx, bootstrap, settings, "settings-r1", []*pluginv1.ActiveGrant{configGrant})
		if err != nil {
			stopFormsDBSession(t, session)
			t.Fatalf("complete Manifest, ConfigSchema and revision-scoped ConfigApply: %v", err)
		}
		if handshake.Manifest.GetName() != "forms-db" || handshake.Manifest.GetProtocolVersion() != sdk.ProtocolVersion {
			stopFormsDBSession(t, session)
			t.Fatalf("unexpected forms-db manifest: %#v", handshake.Manifest)
		}
		if !schemaHasField(handshake.ConfigSchema, "dsn") || !schemaHasField(handshake.ConfigSchema, "schemas") {
			stopFormsDBSession(t, session)
			t.Fatalf("ConfigSchema omitted required settings fields: %#v", handshake.ConfigSchema.GetFields())
		}
		if session.Client().CheckHealth(ctx) == nil {
			stopFormsDBSession(t, session)
			t.Fatal("plugin became healthy before DispatchApply acknowledgement")
		}
		capabilities := make([]*pluginv1.CapabilityDispatchScope, 0, len(handshake.Manifest.GetCapabilityDescriptors()))
		for _, descriptor := range handshake.Manifest.GetCapabilityDescriptors() {
			capabilities = append(capabilities, &pluginv1.CapabilityDispatchScope{
				Capability: descriptor.GetCapability(), Modes: descriptor.GetModes(),
			})
		}
		_, err = session.Client().ApplyDispatch(ctx, &pluginv1.DispatchApplyRequest{
			Generation: 1, InstanceId: formsSmokeInstanceID, SettingsDigest: settingsDigest,
			ReleaseDigest: releaseDigest, Capabilities: capabilities,
		}, formsSmokePluginID)
		if err != nil {
			stopFormsDBSession(t, session)
			t.Fatalf("acknowledge replica-bound DispatchApply: %v", err)
		}
		if err := session.Client().CheckHealth(ctx); err != nil {
			stopFormsDBSession(t, session)
			t.Fatalf("plugin did not become healthy after DispatchApply: %v", err)
		}
		return session, handshake
	}

	session, _ := startSession()
	submission := invokeForms(t, ctx, session, "forms.submit", []byte(`{"site":"smoke","schemaName":"contact","data":{"email":"child@example.test"}}`), nil)
	if submission.Status != 200 {
		stopFormsDBSession(t, session)
		t.Fatalf("forms.submit returned status %d: %s", submission.Status, submission.Body)
	}
	var submitted struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(submission.Body), &submitted); err != nil || submitted.ID == "" {
		stopFormsDBSession(t, session)
		t.Fatalf("forms.submit returned no record ID: body=%s err=%v", submission.Body, err)
	}

	beforeRestart := listForms(t, ctx, session, cursorGrant)
	if len(beforeRestart) != 1 || beforeRestart[0] != submitted.ID {
		stopFormsDBSession(t, session)
		t.Fatalf("forms.list did not return the submitted child-process record: got=%v want=%s", beforeRestart, submitted.ID)
	}
	stopFormsDBSession(t, session)

	// A new process must receive the same opaque DSN reference through a fresh
	// ConfigApply grant and recover the persisted SQLite submission.
	session, _ = startSession()
	afterRestart := listForms(t, ctx, session, cursorGrant)
	if len(afterRestart) != 1 || afterRestart[0] != submitted.ID {
		stopFormsDBSession(t, session)
		t.Fatalf("SQLite state did not survive child restart: got=%v want=%s", afterRestart, submitted.ID)
	}

	deleted := invokeForms(t, ctx, session, "forms.delete", []byte(fmt.Sprintf(`{"site":"smoke","schemaName":"contact","id":%q}`, submitted.ID)), nil)
	if deleted.Status != 200 {
		stopFormsDBSession(t, session)
		t.Fatalf("forms.delete returned status %d: %s", deleted.Status, deleted.Body)
	}
	remaining := listForms(t, ctx, session, cursorGrant)
	if len(remaining) != 0 {
		stopFormsDBSession(t, session)
		t.Fatalf("deleted child-process record is still visible: %v", remaining)
	}
	stopFormsDBSession(t, session)
}

type formsHTTPResult struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

func invokeForms(t *testing.T, ctx context.Context, session *sdk.LocalSession, capability string, payload []byte, grants []*pluginv1.ActiveGrant) formsHTTPResult {
	t.Helper()
	response, err := session.Client().CallWithGrants(ctx, capability, payload, grants)
	if err != nil {
		t.Fatalf("protocol Call %s failed: %v", capability, err)
	}
	var result formsHTTPResult
	if err := json.Unmarshal(response.GetPayload(), &result); err != nil {
		t.Fatalf("decode %s response: %v", capability, err)
	}
	return result
}

func listForms(t *testing.T, ctx context.Context, session *sdk.LocalSession, grant *pluginv1.ActiveGrant) []string {
	t.Helper()
	response := invokeForms(t, ctx, session, "forms.list", []byte(`{"site":"smoke","schemaName":"contact","limit":10}`), []*pluginv1.ActiveGrant{grant})
	if response.Status != 200 {
		t.Fatalf("forms.list returned status %d: %s", response.Status, response.Body)
	}
	var body struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(response.Body), &body); err != nil {
		t.Fatalf("decode forms.list body: %v", err)
	}
	ids := make([]string, 0, len(body.Items))
	for _, item := range body.Items {
		ids = append(ids, item.ID)
	}
	return ids
}

func schemaHasField(schema *pluginv1.ConfigSchema, name string) bool {
	for _, field := range schema.GetFields() {
		if field.GetName() == name {
			return true
		}
	}
	return false
}

func stopFormsDBSession(t *testing.T, session *sdk.LocalSession) {
	t.Helper()
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := session.Stop(stopCtx); err != nil {
		t.Fatalf("stop forms-db child process: %v", err)
	}
}

func formsDBModuleRoot(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate integration test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read built forms-db executable: %v", err)
	}
	return contents
}

func digestBytes(t *testing.T, value []byte) string {
	t.Helper()
	return fmt.Sprintf("sha256:%x", sha256.Sum256(value))
}
