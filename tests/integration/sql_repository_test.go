package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
)

func TestPostgreSQLRepositoryContract(t *testing.T) {
	testSQLRepository(t, "postgres", "FORMS_DB_POSTGRES_DSN", "PostgreSQL")
}

func TestMySQLRepositoryContract(t *testing.T) {
	testSQLRepository(t, "mysql", "FORMS_DB_MYSQL_DSN", "MySQL")
}

func TestMariaDBRepositoryContract(t *testing.T) {
	testSQLRepository(t, "mysql", "FORMS_DB_MARIADB_DSN", "MariaDB")
}

func testSQLRepository(t *testing.T, driver, env, databaseName string) {
	t.Helper()
	dsn := os.Getenv(env)
	if dsn == "" {
		t.Skipf("%s is not set; real %s compatibility was not exercised", env, databaseName)
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
	if err := repository.Close(); err != nil {
		t.Fatalf("close %s repository before restart: %v", driver, err)
	}
	repository, err = storage.NewRepository(ctx, driver, dsn, prefix, map[string]json.RawMessage{
		"contact": json.RawMessage(`{"type":"object","properties":{"email":{"type":"string"},"tag":{"type":"string"}}}`),
	})
	if err != nil {
		t.Fatalf("reopen %s repository: %v", driver, err)
	}
	page, err = repository.List(ctx, "integration", "contact", nil, nil, 10)
	if err != nil || len(page) != 1 || page[0].ID != "frm_b" {
		t.Fatalf("%s persisted rows or scoped delete mismatch after reopen: page=%#v err=%v", driver, page, err)
	}
	testConcurrentRepositoryOperations(t, repository)

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

func testConcurrentRepositoryOperations(t *testing.T, repository *storage.SQLRepository) {
	t.Helper()
	const count = 24
	const site = "concurrent"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := make(chan struct{})
	errorsFound := make(chan string, count+16)
	var workers sync.WaitGroup

	for index := range count {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			id := "concurrent_" + strconv.Itoa(index)
			_, err := repository.Submit(ctx, models.Submission{
				ID: id, Site: site, Schema: "contact", CreatedAt: "2026-02-01T00:00:00Z",
				Data: map[string]any{"email": id + "@example.test"},
			})
			if err != nil {
				errorsFound <- "concurrent submit failed"
			}
		}()
	}
	for reader := 0; reader < 4; reader++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			for range 8 {
				if _, err := repository.List(ctx, site, "contact", nil, nil, 10); err != nil {
					errorsFound <- "concurrent list during submit failed"
					return
				}
			}
		}()
	}
	close(start)
	workers.Wait()
	if len(errorsFound) != 0 {
		t.Fatalf("SQL repository had %d concurrent submit/list failures", len(errorsFound))
	}

	errorsFound = make(chan string, count+16)
	start = make(chan struct{})
	for index := 0; index < count; index += 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			if err := repository.Delete(ctx, site, "contact", "concurrent_"+strconv.Itoa(index)); err != nil {
				errorsFound <- "concurrent delete failed"
			}
		}()
	}
	for reader := 0; reader < 4; reader++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			for range 8 {
				if _, err := repository.List(ctx, site, "contact", nil, nil, 10); err != nil {
					errorsFound <- "concurrent list during delete failed"
					return
				}
			}
		}()
	}
	close(start)
	workers.Wait()
	if len(errorsFound) != 0 {
		t.Fatalf("SQL repository had %d concurrent delete/list failures", len(errorsFound))
	}

	seen := make(map[string]struct{}, count/2)
	var cursor *models.SubmissionCursor
	for {
		items, err := repository.List(ctx, site, "contact", nil, cursor, 7)
		if err != nil {
			t.Fatalf("list concurrent records after workers finished: %v", err)
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			if _, exists := seen[item.ID]; exists {
				t.Fatalf("keyset pagination returned duplicate concurrent record")
			}
			seen[item.ID] = struct{}{}
			index, err := strconv.Atoi(item.ID[len("concurrent_"):])
			if err != nil || index%2 == 0 {
				t.Fatalf("concurrent delete left an unexpected record")
			}
		}
		last := items[len(items)-1]
		cursor = &models.SubmissionCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	if len(seen) != count/2 {
		t.Fatalf("concurrent submit/list/delete left %d records; want %d", len(seen), count/2)
	}
}
