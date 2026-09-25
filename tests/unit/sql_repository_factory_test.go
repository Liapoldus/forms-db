package unit

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Liapoldus/forms-db/internal/domain/models"
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
