package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
)

func TestPostgreSQLRepositoryContract(t *testing.T) {
	testSQLRepository(t, "postgres", "FORMS_DB_POSTGRES_DSN")
}

func TestMySQLRepositoryContract(t *testing.T) {
	testSQLRepository(t, "mysql", "FORMS_DB_MYSQL_DSN")
}

func testSQLRepository(t *testing.T, driver, env string) {
	t.Helper()
	dsn := os.Getenv(env)
	if dsn == "" {
		t.Skipf("%s is not set; real %s compatibility was not exercised", env, driver)
	}
	ctx := context.Background()
	prefix := "formsdb_test_" + strconv.FormatInt(time.Now().UnixNano(), 36) + "_"
	repository, err := storage.NewRepository(ctx, driver, dsn, prefix, map[string]json.RawMessage{
		"contact": json.RawMessage(`{"type":"object","properties":{"email":{"type":"string"},"tag":{"type":"string"}}}`),
	})
	if err != nil {
		t.Fatalf("open %s repository: %v", driver, err)
	}
	t.Cleanup(func() { _ = repository.Close() })

	for _, item := range []models.Submission{
		{ID: "frm_a", Site: "integration", Schema: "contact", CreatedAt: "2026-01-01T00:00:00Z", Data: map[string]any{"email": "one@example.test", "tag": "a"}},
		{ID: "frm_b", Site: "integration", Schema: "contact", CreatedAt: "2026-01-02T00:00:00Z", Data: map[string]any{"email": "two@example.test", "tag": "b"}},
		{ID: "frm_c", Site: "other", Schema: "contact", CreatedAt: "2026-01-03T00:00:00Z", Data: map[string]any{"email": "other@example.test", "tag": "a"}},
		{ID: "frm_d", Site: "filter", Schema: "contact", CreatedAt: "2025-12-30T00:00:00Z", Data: map[string]any{"tag": "null-is-not-missing", "optional": nil}},
		{ID: "frm_e", Site: "filter", Schema: "contact", CreatedAt: "2025-12-31T00:00:00Z", Data: map[string]any{"tag": "missing-is-not-null"}},
	} {
		if _, err := repository.Submit(ctx, item); err != nil {
			t.Fatalf("%s submit: %v", driver, err)
		}
	}

	filter := &models.SubmissionFilter{Field: "email", Equals: json.RawMessage(`"two@example.test"`)}
	page, err := repository.List(ctx, "integration", "contact", filter, nil, 1)
	if err != nil || len(page) != 1 || page[0].ID != "frm_b" {
		t.Fatalf("%s filtered list mismatch: page=%#v err=%v", driver, page, err)
	}
	page, err = repository.List(ctx, "filter", "contact", &models.SubmissionFilter{Field: "optional", Equals: json.RawMessage("null")}, nil, 10)
	if err != nil || len(page) != 1 || page[0].ID != "frm_d" {
		t.Fatalf("%s must distinguish JSON null from a missing property: page=%#v err=%v", driver, page, err)
	}
	page, err = repository.List(ctx, "integration", "contact", &models.SubmissionFilter{Field: "tag", Equals: json.RawMessage(`"b"`)}, nil, 10)
	if err != nil || len(page) != 1 || page[0].ID != "frm_b" {
		t.Fatalf("%s string equality filter mismatch: page=%#v err=%v", driver, page, err)
	}
	page, err = repository.List(ctx, "integration", "contact", nil, nil, 1)
	if err != nil || len(page) != 1 || page[0].ID != "frm_b" {
		t.Fatalf("%s first keyset page mismatch: page=%#v err=%v", driver, page, err)
	}
	page, err = repository.List(ctx, "integration", "contact", nil, &models.SubmissionCursor{CreatedAt: page[0].CreatedAt, ID: page[0].ID}, 10)
	if err != nil || len(page) != 1 || page[0].ID != "frm_a" {
		t.Fatalf("%s second keyset page mismatch: page=%#v err=%v", driver, page, err)
	}
	if err := repository.Delete(ctx, "integration", "contact", "frm_a"); err != nil {
		t.Fatalf("%s scoped delete: %v", driver, err)
	}

	// Verify schema persistence through the configured SQL driver without exposing DSN in failures.
	var sqlDriver string
	if driver == "postgres" {
		sqlDriver = "pgx"
	} else {
		sqlDriver = "mysql"
	}
	db, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		t.Fatalf("open %s schema verification connection: %v", driver, err)
	}
	defer db.Close()
	query := "SELECT schema_json FROM " + prefix + "schemas WHERE site = $1 AND schema_name = $2"
	args := []any{"integration", "contact"}
	if driver == "mysql" {
		query = "SELECT schema_json FROM " + prefix + "schemas WHERE site = ? AND schema_name = ?"
	}
	var schema string
	if err := db.QueryRowContext(ctx, query, args...).Scan(&schema); err != nil || !json.Valid([]byte(schema)) {
		t.Fatalf("%s configured schema was not persisted: %v", driver, err)
	}
}
