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
			return storage.NewSQLiteRepository(ctx, settings.DSN, settings.TablePrefix)
		}
		return storage.NewMemoryRepository(), nil
	}
	server := newCursorTestServerWithBuilder(
		t, application.Service{Repository: storage.NewMemoryRepository()}, builder,
	)
	applySettings(t, server, sqliteSettings(databasePath))

	callFormCapability(t, server, "forms.submit", `{"site":"portal","schemaName":"contact","data":{"name":"persistent"}}`)
	if _, err := server.ConfigApply(ctx, &pluginv1.ConfigApplyRequest{Config: []byte(sqliteSettings(t.TempDir()))}); err == nil {
		t.Fatal("invalid database config must not replace the active repository")
	}
	if count := listFormSubmissions(t, server); count != 1 {
		t.Fatalf("failed config apply changed the active database: count=%d", count)
	}

	applySettings(t, server, sqliteSettings(databasePath))
	if count := listFormSubmissions(t, server); count != 1 {
		t.Fatalf("reopening the configured database lost data: count=%d", count)
	}
}

func sqliteSettings(dsn string) string {
	return fmt.Sprintf(`{"driver":"sqlite","dsn":%q,"tablePrefix":"form_","schemas":{"contact":{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}}}`, dsn)
}

func applySettings(t *testing.T, server *plugin.Server, settings string) {
	t.Helper()
	result, err := server.ConfigApply(context.Background(), &pluginv1.ConfigApplyRequest{Config: []byte(settings)})
	if err != nil || !result.GetApplied() {
		t.Fatalf("config apply failed: result=%#v err=%v", result, err)
	}
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
