package application

import (
	"context"

	"github.com/Liapoldus/forms-db/internal/domain"
)

type Service struct{ Repository domain.Repository }

func (s Service) Submit(ctx context.Context, value domain.Submission) (domain.Submission, error) {
	return s.Repository.Submit(ctx, value)
}

func (s Service) List(ctx context.Context, site, schema string, limit int) ([]domain.Submission, error) {
	return s.Repository.List(ctx, site, schema, limit)
}

func (s Service) Delete(ctx context.Context, id string) error { return s.Repository.Delete(ctx, id) }
