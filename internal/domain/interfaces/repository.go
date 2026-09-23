package interfaces

import (
	"context"

	"github.com/Liapoldus/forms-db/internal/domain/models"
)

// Repository is the seam for future SQLite/PostgreSQL/MySQL adapters.
type Repository interface {
	Submit(context.Context, models.Submission) (models.Submission, error)
	List(context.Context, string, string, *models.SubmissionFilter, int) ([]models.Submission, error)
	Delete(context.Context, string, string, string) error
}
