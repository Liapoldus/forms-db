package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
	_ "modernc.org/sqlite"
)

func main() {
	directory, err := os.MkdirTemp("", "forms-db-timeout-")
	if err != nil {
		panic("create temporary SQLite directory")
	}
	defer os.RemoveAll(directory)
	dsn := filepath.Join(directory, "forms.db") + "?_pragma=busy_timeout%3D10000"
	repository, err := storage.NewRepository(context.Background(), "sqlite", dsn, "form_", map[string]json.RawMessage{
		"contact": json.RawMessage(`{"type":"object","properties":{"email":{"type":"string"}}}`),
	})
	if err != nil {
		panic("open temporary SQLite repository")
	}
	defer repository.Close()
	blocker, err := sql.Open("sqlite", dsn)
	if err != nil {
		panic("open temporary SQLite lock connection")
	}
	defer blocker.Close()
	if _, err := blocker.Exec("BEGIN EXCLUSIVE"); err != nil {
		panic("acquire temporary SQLite write lock")
	}
	started := time.Now()
	_, operationErr := repository.Submit(context.Background(), models.Submission{
		ID: "frm_timeout", Site: "site", Schema: "contact", CreatedAt: "2026-10-03T00:00:00Z",
		Data: map[string]any{"email": "timeout@example.test"},
	})
	elapsed := time.Since(started)
	_, _ = blocker.Exec("ROLLBACK")
	output, _ := json.Marshal(map[string]any{
		"timedOut":            operationErr != nil && elapsed >= 4*time.Second && elapsed < 7*time.Second,
		"elapsedMilliseconds": elapsed.Milliseconds(),
	})
	fmt.Println(string(output))
}
