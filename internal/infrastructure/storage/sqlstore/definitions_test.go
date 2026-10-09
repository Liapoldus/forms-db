package sqlstore_test

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage/memory"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage/sqlstore"
)

func TestStorageListBoundsAndSubmissionIdentifiers(t *testing.T) {
	for _, backend := range []string{"memory", "sqlite"} {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			var repository interface {
				interfaces.Repository
				Close() error
			} = memory.New()
			if backend == "sqlite" {
				var err error
				repository, err = sqlstore.NewRepository(ctx, "sqlite", filepath.Join(t.TempDir(), "forms.db"), "", nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() {
				if err := repository.Close(); err != nil {
					t.Fatal(err)
				}
			})
			for range 110 {
				item, err := repository.Submit(ctx, models.Submission{Site: "site", Schema: "contact", CreatedAt: "2026-01-01T00:00:00Z", Data: map[string]any{}})
				if err != nil {
					t.Fatal(err)
				}
				if !strings.HasPrefix(item.ID, "frm_") {
					t.Fatalf("unexpected ID prefix: %q", item.ID)
				}
				random, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(item.ID, "frm_"))
				if err != nil || len(random) != 18 {
					t.Fatal("submission ID must contain 18 random bytes")
				}
			}
			for _, limit := range []int{-1, 0, 1, 50, 101, 102, 1000} {
				want := limit
				if want < 1 {
					want = 50
				}
				if want > 101 {
					want = 101
				}
				items, err := repository.List(ctx, "site", "contact", nil, nil, limit)
				if err != nil || len(items) != want {
					t.Fatalf("limit %d: got %d rows, err %v; want %d", limit, len(items), err, want)
				}
			}
		})
	}
}

func TestSQLFilteredCursorScansBeyondUnfilteredLimit(t *testing.T) {
	ctx := context.Background()
	repository, err := sqlstore.NewRepository(ctx, "sqlite", filepath.Join(t.TempDir(), "forms.db"), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := repository.Close(); err != nil {
			t.Fatal(err)
		}
	})
	for i := range 110 {
		_, err := repository.Submit(ctx, models.Submission{ID: fmt.Sprintf("frm_%03d", i), Site: "site", Schema: "contact", CreatedAt: "2026-01-01T00:00:00Z", Data: map[string]any{"match": i < 2}})
		if err != nil {
			t.Fatal(err)
		}
	}
	filter := &models.SubmissionFilter{Field: "match", Equals: json.RawMessage(`true`)}
	page, err := repository.List(ctx, "site", "contact", filter, nil, 1)
	if err != nil || len(page) != 1 || page[0].ID != "frm_001" {
		t.Fatalf("filtered first page: %#v, %v", page, err)
	}
	after := &models.SubmissionCursor{CreatedAt: page[0].CreatedAt, ID: page[0].ID}
	page, err = repository.List(ctx, "site", "contact", filter, after, 1)
	if err != nil || len(page) != 1 || page[0].ID != "frm_000" {
		t.Fatalf("filtered cursor page: %#v, %v", page, err)
	}
}

func TestSQLDurableNamesAndSanitizedMessages(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "forms.db")
	repository, err := sqlstore.NewRepository(ctx, "sqlite", dsn, "", map[string]json.RawMessage{"contact": json.RawMessage(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { checkError(t, repository.Close()) })
	item := models.Submission{ID: "frm_private_payload", Site: "site", Schema: "contact", Data: map[string]any{"private": "secret-value"}}
	if _, err := repository.Submit(ctx, item); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { checkError(t, db.Close()) })
	for _, table := range []struct {
		name    string
		columns string
	}{
		{"form_schemas", "site,schema_name,schema_json,updated_at"},
		{"form_submissions", "id,site,schema_name,created_at,data_json"},
	} {
		rows, err := db.QueryContext(ctx, "PRAGMA table_info("+table.name+")")
		if err != nil {
			t.Fatal(err)
		}
		var columns []string
		defer func() { checkError(t, rows.Close()) }()
		for rows.Next() {
			var cid, notNull, primaryKey int
			var name, dataType string
			var defaultValue any
			if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
				t.Fatal(err)
			}
			columns = append(columns, name)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		checkError(t, rows.Close())
		if strings.Join(columns, ",") != table.columns {
			t.Fatalf("durable columns changed for %s: %v", table.name, columns)
		}
	}
	var indexDDL string
	if err := db.QueryRowContext(ctx, "SELECT sql FROM sqlite_master WHERE type = 'index' AND name = 'form_submissions_scope_created_idx'").Scan(&indexDDL); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(indexDDL, "(`site`, `schema_name`, `created_at`, `id`)") {
		t.Fatalf("scope/cursor index changed: %s", indexDDL)
	}
	if _, err := repository.Submit(ctx, item); err == nil || err.Error() != "store submission" {
		t.Fatalf("duplicate error must stay sanitized: %v", err)
	}
	item.Data = map[string]any{"invalid": make(chan int)}
	if _, err := repository.Submit(ctx, item); err == nil || err.Error() != "encode submission data" {
		t.Fatalf("encoding error must stay sanitized: %v", err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.List(ctx, "site", "contact", nil, nil, 1); err == nil || err.Error() != "list submissions" {
		t.Fatalf("closed list error changed: %v", err)
	}
	if err := repository.Delete(ctx, "site", "contact", item.ID); err == nil || err.Error() != "delete submission" {
		t.Fatalf("closed delete error changed: %v", err)
	}
}
