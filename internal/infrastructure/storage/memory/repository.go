// Package memory implements volatile form persistence.
package memory

import (
	"context"
	"encoding/json"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage/record"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/domain/models"
)

type Repository struct {
	mu          sync.Mutex
	submissions []models.Submission
}

func New() *Repository { return &Repository{} }

func (r *Repository) Submit(_ context.Context, submission models.Submission) (models.Submission, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if submission.ID == "" {
		id, err := record.NewID()
		if err != nil {
			return models.Submission{}, err
		}
		submission.ID = id
	}
	if submission.CreatedAt == "" {
		submission.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	r.submissions = append(r.submissions, submission)
	return submission, nil
}

func (r *Repository) Close() error { return nil }

func (r *Repository) List(_ context.Context, site, schema string, filter *models.SubmissionFilter, after *models.SubmissionCursor, limit int) ([]models.Submission, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit < 1 {
		limit = record.DefaultListRows
	}
	if limit > record.MaxListRows {
		limit = record.MaxListRows
	}
	candidates := make([]models.Submission, 0, len(r.submissions))
	for _, item := range r.submissions {
		if item.Site == site && item.Schema == schema && matchesFilter(item, filter) && isAfterCursor(item, after) {
			candidates = append(candidates, item)
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].CreatedAt == candidates[right].CreatedAt {
			return candidates[left].ID > candidates[right].ID
		}
		return candidates[left].CreatedAt > candidates[right].CreatedAt
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, nil
}

func isAfterCursor(item models.Submission, after *models.SubmissionCursor) bool {
	return after == nil || item.CreatedAt < after.CreatedAt || (item.CreatedAt == after.CreatedAt && item.ID < after.ID)
}

func matchesFilter(item models.Submission, filter *models.SubmissionFilter) bool {
	if filter == nil {
		return true
	}
	actual, exists := item.Data[filter.Field]
	if !exists {
		return false
	}
	var expected any
	if json.Unmarshal(filter.Equals, &expected) != nil {
		return false
	}
	return reflect.DeepEqual(actual, expected)
}

func (r *Repository) Delete(_ context.Context, site, schema, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index, item := range r.submissions {
		if item.Site == site && item.Schema == schema && item.ID == id {
			r.submissions = append(r.submissions[:index], r.submissions[index+1:]...)
			return nil
		}
	}
	return interfaces.ErrNotFound
}
