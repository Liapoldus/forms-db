package sqlstore

import (
	"context"
	"database/sql"

	"encoding/json"
	"errors"
	"fmt"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage/record"

	"time"

	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/domain/models"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

func (r *Repository) Submit(ctx context.Context, submission models.Submission) (models.Submission, error) {
	ctx, release, err := r.beginOperation(ctx)
	if err != nil {
		return models.Submission{}, err
	}
	defer release()
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
	data, err := json.Marshal(submission.Data)
	if err != nil {
		return models.Submission{}, errors.New(messageEncodeSubmissionData)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return models.Submission{}, errors.New(messageBeginSubmissionTransaction)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			// The original transaction error is already sanitized and takes precedence.
			return
		}
	}()
	if schema, exists := r.schemas[submission.Schema]; exists {
		if err := r.upsertSchema(ctx, tx, submission.Site, submission.Schema, schema); err != nil {
			return models.Submission{}, err
		}
	}
	query := fmt.Sprintf(sqlInsertSubmission,
		r.submissionsTable,
		quoteIdentifier(r.dialect, idColumnName), quoteIdentifier(r.dialect, siteColumnName),
		quoteIdentifier(r.dialect, schemaNameColumnName), quoteIdentifier(r.dialect, createdAtColumnName),
		quoteIdentifier(r.dialect, dataJSONColumnName), r.bind(1), r.bind(2), r.bind(3), r.bind(4), r.bind(5),
	)
	if _, err := tx.ExecContext(ctx, query, submission.ID, submission.Site, submission.Schema, submission.CreatedAt, string(data)); err != nil {
		return models.Submission{}, errors.New(messageStoreSubmission)
	}
	if err := tx.Commit(); err != nil {
		return models.Submission{}, errors.New(messageCommitSubmissionTransaction)
	}
	return submission, nil
}

func (r *Repository) Delete(ctx context.Context, site, schema, id string) error {
	ctx, release, err := r.beginOperation(ctx)
	if err != nil {
		return err
	}
	defer release()
	query := fmt.Sprintf(sqlDeleteSubmission,
		r.submissionsTable,
		quoteIdentifier(r.dialect, siteColumnName), r.bind(1),
		quoteIdentifier(r.dialect, schemaNameColumnName), r.bind(2),
		quoteIdentifier(r.dialect, idColumnName), r.bind(3),
	)
	result, err := r.db.ExecContext(ctx, query, site, schema, id)
	if err != nil {
		return errors.New(messageDeleteSubmission)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return errors.New(messageCheckDeletedSubmission)
	}
	if deleted == 0 {
		return interfaces.ErrNotFound
	}
	return nil
}
