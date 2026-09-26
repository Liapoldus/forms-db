package interfaces

import (
	"context"
	"errors"

	"github.com/Liapoldus/forms-db/internal/domain/models"
)

var ErrNotFound = errors.New("submission not found")

// Repository is the seam for future SQLite/PostgreSQL/MySQL adapters.
type Repository interface {
	Submit(context.Context, models.Submission) (models.Submission, error)
	List(context.Context, string, string, *models.SubmissionFilter, *models.SubmissionCursor, int) ([]models.Submission, error)
	Delete(context.Context, string, string, string) error
}
