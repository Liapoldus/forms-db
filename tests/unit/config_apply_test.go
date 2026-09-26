package unit

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
	"github.com/Liapoldus/forms-db/internal/presentation/plugin"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
)

func TestConfigApplySwitchesToSQLiteAndRetainsActiveDatabaseOnFailure(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "forms.db")
	builder := func(ctx context.Context, settings config.Settings) (interfaces.Repository, error) {
		if settings.Driver == "sqlite" {
			if string(settings.DSN) == "reject-dsn" {
				return nil, fmt.Errorf("test repository setup rejected")
			}
			return storage.NewSQLiteRepository(ctx, string(settings.DSN), settings.TablePrefix)
		}
		return storage.NewMemoryRepository(), nil
	}
	server := newCursorTestServerWithBuilderAndConfigSecrets(
		t, application.Service{Repository: storage.NewMemoryRepository()}, builder,
		map[string][]byte{"sqlite-dsn-reference": []byte(databasePath), "failing-dsn-reference": []byte("reject-dsn")},
	)
	applySettings(t, server, sqliteSettings("sqlite-dsn-reference"))

	callFormCapability(t, server, "forms.submit", `{"site":"portal","schemaName":"contact","data":{"name":"persistent"}}`)
	if _, err := server.ConfigApply(ctx, configApplyRequest(sqliteSettings("failing-dsn-reference"))); err == nil {
		t.Fatal("invalid database config must not replace the active repository")
	}
	if count := listFormSubmissions(t, server); count != 1 {
		t.Fatalf("failed config apply changed the active database: count=%d", count)
	}

	applySettings(t, server, sqliteSettings("sqlite-dsn-reference"))
	if count := listFormSubmissions(t, server); count != 1 {
		t.Fatalf("reopening the configured database lost data: count=%d", count)
	}
}

func TestConfigApplyRejectsUnmatchedOrRawDSNReference(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "forms.db")
	builder := func(ctx context.Context, settings config.Settings) (interfaces.Repository, error) {
		return storage.NewSQLiteRepository(ctx, string(settings.DSN), settings.TablePrefix)
	}
	server := newCursorTestServerWithBuilderAndConfigSecrets(
		t, application.Service{Repository: storage.NewMemoryRepository()}, builder,
		map[string][]byte{"valid-dsn-reference": []byte(databasePath)},
	)
	valid := configApplyRequest(sqliteSettings("valid-dsn-reference"))
	if result, err := server.ConfigApply(context.Background(), valid); err != nil || !result.GetApplied() {
		t.Fatalf("valid reference-bound DSN grant must apply: result=%#v err=%v", result, err)
	}

	for _, invalid := range []*pluginv1.ConfigApplyRequest{
		configApplyRequest(sqliteSettings(databasePath)),
		wrongRevisionConfigRequest(sqliteSettings("valid-dsn-reference")),
	} {
		if result, err := server.ConfigApply(context.Background(), invalid); err == nil || result.GetApplied() {
			t.Fatalf("unmatched DSN grant must not apply: result=%#v err=%v", result, err)
		}
	}
	if count := listFormSubmissions(t, server); count != 0 {
		t.Fatalf("failed config applies must preserve the active repository: count=%d", count)
	}
}

func wrongRevisionConfigRequest(raw string) *pluginv1.ConfigApplyRequest {
	request := configApplyRequest(raw)
	request.SettingsRevision += "-changed"
	return request
}

func sqliteSettings(dsn string) string {
	return fmt.Sprintf(`{"driver":"sqlite","dsn":%q,"tablePrefix":"form_","schemas":{"contact":{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}}}`, dsn)
}

func applySettings(t *testing.T, server *plugin.Server, settings string) {
	t.Helper()
	applyTestConfig(t, server, settings)
}

func callFormCapability(t *testing.T, server *plugin.Server, capability, payload string) {
	t.Helper()
	response, err := server.Call(context.Background(), &pluginv1.CallRequest{Capability: capability, Payload: []byte(payload)})
	if err != nil {
		t.Fatal(err)
	}
	var httpResponse struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(response.GetPayload(), &httpResponse); err != nil {
		t.Fatal(err)
	}
	if httpResponse.Status != 200 {
		t.Fatalf("capability returned status %d", httpResponse.Status)
	}
}

func listFormSubmissions(t *testing.T, server *plugin.Server) int {
	t.Helper()
	response, err := server.Call(context.Background(), &pluginv1.CallRequest{
		Capability: "forms.list",
		Payload:    []byte(`{"site":"portal","schemaName":"contact","limit":50}`),
		Grants:     []*pluginv1.ActiveGrant{testCursorGrant(t)},
	})
	if err != nil {
		t.Fatal(err)
	}
	var httpResponse struct {
		Status int    `json:"status"`
		Body   []byte `json:"body"`
	}
	if err := json.Unmarshal(response.GetPayload(), &httpResponse); err != nil {
		t.Fatal(err)
	}
	if httpResponse.Status != 200 {
		t.Fatalf("forms.list returned status %d", httpResponse.Status)
	}
	var result struct {
		Items []models.Submission `json:"items"`
	}
	if err := json.Unmarshal(httpResponse.Body, &result); err != nil {
		t.Fatal(err)
	}
	return len(result.Items)
}
