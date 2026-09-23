package unit

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
)

func TestSQLiteRepositoryPersistsOrderedSiteScopedSubmissions(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "forms.db")
	repository, err := storage.NewSQLiteRepository(ctx, databasePath, "form_")
	if err != nil {
		t.Fatal(err)
	}

	older, err := repository.Submit(ctx, models.Submission{
		Site: "portal", Schema: "contact", CreatedAt: "2026-01-01T00:00:00Z",
		Data: map[string]any{"name": "older"},
	})
	if err != nil {
		t.Fatal(err)
	}
	newer, err := repository.Submit(ctx, models.Submission{
		Site: "portal", Schema: "contact", CreatedAt: "2026-01-02T00:00:00Z",
		Data: map[string]any{"name": "newer"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Submit(ctx, models.Submission{
		Site: "other", Schema: "contact", CreatedAt: "2026-01-03T00:00:00Z",
		Data: map[string]any{"name": "other-site"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}

	repository, err = storage.NewSQLiteRepository(ctx, databasePath, "form_")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	items, err := repository.List(ctx, "portal", "contact", nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != newer.ID || items[1].ID != older.ID {
		t.Fatalf("expected persisted newest-first scoped results, got %#v", items)
	}

	if err := repository.Delete(ctx, "other", "contact", newer.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("delete outside the submission scope must be not-found, got %v", err)
	}
	if err := repository.Delete(ctx, "portal", "contact", newer.ID); err != nil {
		t.Fatal(err)
	}
	items, err = repository.List(ctx, "portal", "contact", nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != older.ID {
		t.Fatalf("expected only the older submission after scoped delete, got %#v", items)
	}
}

func TestSQLiteRepositoryFiltersByTopLevelFieldEquality(t *testing.T) {
	ctx := context.Background()
	repository, err := storage.NewSQLiteRepository(ctx, filepath.Join(t.TempDir(), "forms.db"), "form_")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	for index, email := range []string{"one@example.test", "two@example.test"} {
		data := map[string]any{"email": email, "metadata": map[string]any{"source": "ad"}}
		if index == 0 {
			data["optional"] = nil
		}
		if _, err := repository.Submit(ctx, models.Submission{
			Site: "portal", Schema: "contact", Data: data,
		}); err != nil {
			t.Fatal(err)
		}
	}

	items, err := repository.List(ctx, "portal", "contact", &models.SubmissionFilter{
		Field: "email", Equals: []byte(`"one@example.test"`),
	}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Data["email"] != "one@example.test" {
		t.Fatalf("expected only the row matching the JSON equality filter, got %#v", items)
	}

	items, err = repository.List(ctx, "portal", "contact", &models.SubmissionFilter{
		Field: "metadata", Equals: []byte(`{"source": "ad"}`),
	}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("object equality must ignore JSON whitespace, got %#v", items)
	}

	items, err = repository.List(ctx, "portal", "contact", &models.SubmissionFilter{
		Field: "optional", Equals: []byte("null"),
	}, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Data["email"] != "one@example.test" {
		t.Fatalf("null equality must match explicit null but not a missing property, got %#v", items)
	}
}
