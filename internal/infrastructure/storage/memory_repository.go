package storage

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Liapoldus/forms-db/internal/domain/models"
)

var ErrNotFound = errors.New("submission not found")

type MemoryRepository struct {
	mu          sync.Mutex
	submissions []models.Submission
}

func NewMemoryRepository() *MemoryRepository { return &MemoryRepository{} }

func (r *MemoryRepository) Submit(_ context.Context, submission models.Submission) (models.Submission, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if submission.ID == "" {
		id, err := newSubmissionID()
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

func (r *MemoryRepository) Close() error { return nil }

func (r *MemoryRepository) List(_ context.Context, site, schema string, limit int) ([]models.Submission, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit < 1 {
		limit = 50
	}
	result := make([]models.Submission, 0, limit)
	for index := len(r.submissions) - 1; index >= 0 && len(result) < limit; index-- {
		item := r.submissions[index]
		if item.Site == site && item.Schema == schema {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *MemoryRepository) Delete(_ context.Context, site, schema, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index, item := range r.submissions {
		if item.Site == site && item.Schema == schema && item.ID == id {
			r.submissions = append(r.submissions[:index], r.submissions[index+1:]...)
			return nil
		}
	}
	return ErrNotFound
}
