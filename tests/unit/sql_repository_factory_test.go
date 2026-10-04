package unit

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
)

func TestRepositoryFactoryPersistsSchemaAndSubmission(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "forms.db")
	schemas := map[string]json.RawMessage{"contact": json.RawMessage(`{"type":"object","properties":{"email":{"type":"string"}}}`)}
	repository, err := storage.NewRepository(ctx, "sqlite", dsn, "forms_", schemas)
	if err != nil {
		t.Fatal(err)
	}
	item, err := repository.Submit(ctx, models.Submission{
		ID: "frm_one", Site: "portal", Schema: "contact", CreatedAt: "2026-01-02T03:04:05Z",
		Data: map[string]any{"email": "one@example.test"},
	})
	if err != nil || item.ID != "frm_one" {
		t.Fatalf("submit failed: item=%#v err=%v", item, err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var storedSchema string
	if err := db.QueryRowContext(ctx, "SELECT schema_json FROM forms_schemas WHERE site = ? AND schema_name = ?", "portal", "contact").Scan(&storedSchema); err != nil {
		t.Fatalf("configured schema was not persisted: %v", err)
	}
	if !json.Valid([]byte(storedSchema)) {
		t.Fatalf("persisted schema is not JSON: %q", storedSchema)
	}
}

func TestRepositoryFactoryRejectsUnsafeTablePrefix(t *testing.T) {
	_, err := storage.NewRepository(context.Background(), "sqlite", filepath.Join(t.TempDir(), "forms.db"), "x`; DROP TABLE submissions;--", nil)
	if err == nil {
		t.Fatal("unsafe table prefix must be rejected")
	}
}

func TestRepositoryFactoryPersistsConfiguredSchemaWithSubmissionAtomically(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "forms.db")
	oldSchema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}}}`)
	repository, err := storage.NewRepository(ctx, "sqlite", dsn, "forms_", map[string]json.RawMessage{"contact": oldSchema})
	if err != nil {
		t.Fatal("open configured SQLite repository")
	}
	if _, err := repository.Submit(ctx, models.Submission{
		ID: "frm_existing", Site: "portal", Schema: "contact", CreatedAt: "2026-01-01T00:00:00Z", Data: map[string]any{"name": "saved"},
	}); err != nil {
		t.Fatal("persist submission and schema")
	}
	if err := repository.Close(); err != nil {
		t.Fatal("close configured SQLite repository")
	}

	newSchema := json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"phone":{"type":"string"}}}`)
	repository, err = storage.NewRepository(ctx, "sqlite", dsn, "forms_", map[string]json.RawMessage{"contact": newSchema})
	if err != nil {
		t.Fatal("reopen SQLite repository with candidate schema")
	}
	defer repository.Close()
	if _, err := repository.Submit(ctx, models.Submission{
		ID: "frm_existing", Site: "portal", Schema: "contact", CreatedAt: "2026-01-02T00:00:00Z", Data: map[string]any{"name": "duplicate"},
	}); err == nil {
		t.Fatal("duplicate submission must fail")
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal("open SQLite schema verification connection")
	}
	defer db.Close()
	var storedSchema string
	if err := db.QueryRowContext(ctx, "SELECT schema_json FROM forms_schemas WHERE site = ? AND schema_name = ?", "portal", "contact").Scan(&storedSchema); err != nil {
		t.Fatal("read persisted schema")
	}
	if storedSchema != string(oldSchema) {
		t.Fatalf("failed submission must roll back its candidate schema update: got %s", storedSchema)
	}
}

func TestRepositoryFactoryUsesStructuralJSONEqualityAndDistinguishesMissingFromNull(t *testing.T) {
	ctx := context.Background()
	repository, err := storage.NewRepository(ctx, "sqlite", filepath.Join(t.TempDir(), "forms.db"), "forms_", map[string]json.RawMessage{
		"contact": json.RawMessage(`{"type":"object"}`),
	})
	if err != nil {
		t.Fatal("open SQLite repository")
	}
	defer repository.Close()
	for _, item := range []models.Submission{
		{ID: "frm_object", Site: "portal", Schema: "contact", CreatedAt: "2026-01-03T00:00:00Z", Data: map[string]any{"value": map[string]any{"b": 2.0, "a": []any{true, "x"}}}},
		{ID: "frm_null", Site: "portal", Schema: "contact", CreatedAt: "2026-01-02T00:00:00Z", Data: map[string]any{"value": nil}},
		{ID: "frm_missing", Site: "portal", Schema: "contact", CreatedAt: "2026-01-01T00:00:00Z", Data: map[string]any{}},
	} {
		if _, err := repository.Submit(ctx, item); err != nil {
			t.Fatal("submit JSON fixture")
		}
	}

	page, err := repository.List(ctx, "portal", "contact", &models.SubmissionFilter{
		Field: "value", Equals: json.RawMessage(`{"a":[true,"x"],"b":2}`),
	}, nil, 10)
	if err != nil || len(page) != 1 || page[0].ID != "frm_object" {
		t.Fatalf("object equality must ignore key order and insignificant number formatting: page=%#v err=%v", page, err)
	}
	page, err = repository.List(ctx, "portal", "contact", &models.SubmissionFilter{
		Field: "value", Equals: json.RawMessage("null"),
	}, nil, 10)
	if err != nil || len(page) != 1 || page[0].ID != "frm_null" {
		t.Fatalf("JSON null equality must exclude a missing property: page=%#v err=%v", page, err)
	}
}

func TestConfigurationValidationAcceptsSQLAdaptersAndValidatesTablePrefix(t *testing.T) {
	for _, driver := range []string{"postgres", "mysql"} {
		settings, err := config.Apply([]byte(`{"driver":"` + driver + `","dsn":"configured-secret","tablePrefix":"forms_"}`))
		if err != nil || settings.Driver != driver {
			t.Fatalf("configuration validation must accept the documented %s adapter: settings=%#v err=%v", driver, settings, err)
		}
	}
	for _, prefix := range []string{"", "x`; DROP TABLE submissions;--", "1forms_", "forms.schema."} {
		if prefix == "" {
			continue
		}
		if _, err := config.Apply([]byte(`{"driver":"sqlite","tablePrefix":` + mustJSON(t, prefix) + `}`)); err == nil {
			t.Fatalf("unsafe table prefix %q must be rejected", prefix)
		}
	}
}

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal("encode test value")
	}
	return string(encoded)
}
