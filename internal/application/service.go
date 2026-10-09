// Package application coordinates form submission operations.
package application

import (
	"context"

	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/domain/models"
)

type Service struct{ Repository interfaces.Repository }

func (s Service) Submit(ctx context.Context, value models.Submission) (models.Submission, error) {
	return s.Repository.Submit(ctx, value)
}

func (s Service) List(ctx context.Context, site, schema string, filter *models.SubmissionFilter, after *models.SubmissionCursor, limit int) ([]models.Submission, error) {
	return s.Repository.List(ctx, site, schema, filter, after, limit)
}

func (s Service) Delete(ctx context.Context, site, schema, id string) error {
	return s.Repository.Delete(ctx, site, schema, id)
}
