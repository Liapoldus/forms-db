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

	items, err := repository.List(ctx, "portal", "contact", nil, nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != newer.ID || items[1].ID != older.ID {
		t.Fatalf("expected persisted newest-first scoped results, got %#v", items)
	}
	page, err := repository.List(ctx, "portal", "contact", nil, nil, 1)
	if err != nil || len(page) != 1 || page[0].ID != newer.ID {
		t.Fatalf("first keyset page should contain newest item: page=%#v err=%v", page, err)
	}
	page, err = repository.List(ctx, "portal", "contact", nil, &models.SubmissionCursor{CreatedAt: page[0].CreatedAt, ID: page[0].ID}, 1)
	if err != nil || len(page) != 1 || page[0].ID != older.ID {
		t.Fatalf("second keyset page should contain older item: page=%#v err=%v", page, err)
	}

	if err := repository.Delete(ctx, "other", "contact", newer.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("delete outside the submission scope must be not-found, got %v", err)
	}
	if err := repository.Delete(ctx, "portal", "contact", newer.ID); err != nil {
		t.Fatal(err)
	}
	items, err = repository.List(ctx, "portal", "contact", nil, nil, 50)
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
	}, nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Data["email"] != "one@example.test" {
		t.Fatalf("expected only the row matching the JSON equality filter, got %#v", items)
	}

	items, err = repository.List(ctx, "portal", "contact", &models.SubmissionFilter{
		Field: "metadata", Equals: []byte(`{"source": "ad"}`),
	}, nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("object equality must ignore JSON whitespace, got %#v", items)
	}

	items, err = repository.List(ctx, "portal", "contact", &models.SubmissionFilter{
		Field: "optional", Equals: []byte("null"),
	}, nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Data["email"] != "one@example.test" {
		t.Fatalf("null equality must match explicit null but not a missing property, got %#v", items)
	}
}

func TestSQLiteRepositoryKeysetPagesDoNotDuplicateEqualTimestamps(t *testing.T) {
	ctx := context.Background()
	repository, err := storage.NewSQLiteRepository(ctx, filepath.Join(t.TempDir(), "forms.db"), "form_")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	for _, id := range []string{"frm_a", "frm_b", "frm_c"} {
		if _, err := repository.Submit(ctx, models.Submission{
			ID: id, CreatedAt: "2026-04-05T06:07:08Z", Site: "portal", Schema: "contact", Data: map[string]any{},
		}); err != nil {
			t.Fatal("store tied timestamp submission")
		}
	}
	var found []string
	var position *models.SubmissionCursor
	for range 3 {
		page, err := repository.List(ctx, "portal", "contact", nil, position, 1)
		if err != nil || len(page) != 1 {
			t.Fatalf("one-item keyset page failed: page=%#v err=%v", page, err)
		}
		found = append(found, page[0].ID)
		position = &models.SubmissionCursor{CreatedAt: page[0].CreatedAt, ID: page[0].ID}
	}
	if len(found) != 3 || found[0] != "frm_c" || found[1] != "frm_b" || found[2] != "frm_a" {
		t.Fatalf("tied timestamps must be traversed once in ID-descending order: %#v", found)
	}
}
