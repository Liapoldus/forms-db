package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/security"
	"github.com/Liapoldus/forms-db/internal/presentation/plugin"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
)

const testPluginInstanceID = "forms-db-test-instance"

var settingsRevisionSequence atomic.Uint64

func bootstrapTestServer(t *testing.T, server *plugin.Server, redeemer plugin.GrantRedeemer) {
	t.Helper()
	if redeemer != nil {
		server.SetGrantBrokerDialer(func(context.Context, *pluginv1.BootstrapRequest) (plugin.GrantRedeemer, error) {
			return redeemer, nil
		})
	}
	request := &pluginv1.BootstrapRequest{InstanceId: testPluginInstanceID}
	if redeemer != nil {
		request.GrantBrokerEndpoint = "127.0.0.1:1"
	}
	if result, err := server.Bootstrap(context.Background(), request); err != nil || !result.GetAccepted() {
		t.Fatalf("test plugin bootstrap failed: result=%#v err=%v", result, err)
	}
}

func newBootstrappedTestServer(t *testing.T, service application.Service) *plugin.Server {
	t.Helper()
	server := plugin.NewServer(service, nil)
	bootstrapTestServer(t, server, nil)
	return server
}

func configApplyRequest(raw string) *pluginv1.ConfigApplyRequest {
	revision := fmt.Sprintf("forms-db-test-revision-%d", settingsRevisionSequence.Add(1))
	request := &pluginv1.ConfigApplyRequest{Config: []byte(raw), SettingsRevision: revision}
	var settings struct {
		DSNReference string `json:"dsn"`
	}
	if json.Unmarshal(request.Config, &settings) == nil && settings.DSNReference != "" {
		purpose, ok := config.DSNSecretGrantPurpose()
		if ok {
			request.Grants = []*pluginv1.ActiveGrant{{
				Handle:           "test-config-grant",
				Purpose:          purpose,
				Scope:            pluginv1.GrantScope_GRANT_SCOPE_CONFIG_APPLY,
				InstanceId:       testPluginInstanceID,
				SettingsRevision: revision,
				SecretReference:  settings.DSNReference,
			}}
		}
	}
	return request
}

func applyTestConfig(t *testing.T, server *plugin.Server, raw string) {
	t.Helper()
	request := configApplyRequest(raw)
	result, err := server.ConfigApply(context.Background(), request)
	if err != nil || !result.GetApplied() || result.GetSettingsRevision() != request.GetSettingsRevision() {
		t.Fatalf("config apply failed: result=%#v err=%v", result, err)
	}
}

func testCursorGrant(t *testing.T) *pluginv1.ActiveGrant {
	t.Helper()
	capability, purpose, domain, ok := security.CursorGrantScope()
	if !ok {
		t.Fatal("cursor grant contract unavailable")
	}
	return &pluginv1.ActiveGrant{
		Handle: "test-handle", Capability: capability, Purpose: purpose,
		Domains: []string{domain}, Scope: pluginv1.GrantScope_GRANT_SCOPE_CALL,
	}
}
