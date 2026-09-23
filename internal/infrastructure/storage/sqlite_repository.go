package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/domain/models"
	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	db               *sql.DB
	contract         storageContract
	schemasTable     string
	submissionsTable string
	submissionIndex  string
}

func NewSQLiteRepository(ctx context.Context, dsn, tablePrefix string) (*SQLiteRepository, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("SQLite DSN is required")
	}
	contract, err := loadStorageContract()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, errors.New("open SQLite repository")
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, errors.New("connect SQLite repository")
	}

	repository := &SQLiteRepository{
		contract:         contract,
		db:               db,
		schemasTable:     quoteIdentifier(tablePrefix + contract.Tables.Schemas),
		submissionsTable: quoteIdentifier(tablePrefix + contract.Tables.Submissions),
		submissionIndex:  quoteIdentifier(tablePrefix + contract.Indexes.SubmissionScopeCreatedAt),
	}
	if err := repository.initialize(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return repository, nil
}

func (r *SQLiteRepository) initialize(ctx context.Context) error {
	statements := []string{
		fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s TEXT NOT NULL, %s TEXT NOT NULL, %s TEXT NOT NULL, %s TEXT NOT NULL, PRIMARY KEY (%s, %s))",
			r.schemasTable, quoteIdentifier(r.contract.Columns.Site), quoteIdentifier(r.contract.Columns.SchemaName),
			quoteIdentifier(r.contract.Columns.SchemaJSON), quoteIdentifier(r.contract.Columns.UpdatedAt),
			quoteIdentifier(r.contract.Columns.Site), quoteIdentifier(r.contract.Columns.SchemaName)),
		fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s TEXT PRIMARY KEY, %s TEXT NOT NULL, %s TEXT NOT NULL, %s TEXT NOT NULL, %s TEXT NOT NULL)",
			r.submissionsTable, quoteIdentifier(r.contract.Columns.ID), quoteIdentifier(r.contract.Columns.Site),
			quoteIdentifier(r.contract.Columns.SchemaName), quoteIdentifier(r.contract.Columns.CreatedAt), quoteIdentifier(r.contract.Columns.DataJSON)),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (%s, %s, %s DESC, %s DESC)",
			r.submissionIndex, r.submissionsTable, quoteIdentifier(r.contract.Columns.Site),
			quoteIdentifier(r.contract.Columns.SchemaName), quoteIdentifier(r.contract.Columns.CreatedAt), quoteIdentifier(r.contract.Columns.ID)),
	}
	for _, statement := range statements {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return errors.New("initialize SQLite repository")
		}
	}
	return nil
}

func (r *SQLiteRepository) Submit(ctx context.Context, submission models.Submission) (models.Submission, error) {
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
	data, err := json.Marshal(submission.Data)
	if err != nil {
		return models.Submission{}, errors.New("encode submission data")
	}
	_, err = r.db.ExecContext(ctx,
		fmt.Sprintf("INSERT INTO %s (%s, %s, %s, %s, %s) VALUES (?, ?, ?, ?, ?)", r.submissionsTable,
			quoteIdentifier(r.contract.Columns.ID), quoteIdentifier(r.contract.Columns.Site), quoteIdentifier(r.contract.Columns.SchemaName),
			quoteIdentifier(r.contract.Columns.CreatedAt), quoteIdentifier(r.contract.Columns.DataJSON)),
		submission.ID, submission.Site, submission.Schema, submission.CreatedAt, string(data),
	)
	if err != nil {
		return models.Submission{}, errors.New("store submission")
	}
	return submission, nil
}

func (r *SQLiteRepository) List(ctx context.Context, site, schema string, filter *models.SubmissionFilter, limit int) ([]models.Submission, error) {
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	query := fmt.Sprintf("SELECT %s, %s, %s, %s, %s FROM %s WHERE %s = ? AND %s = ?",
		quoteIdentifier(r.contract.Columns.ID), quoteIdentifier(r.contract.Columns.CreatedAt), quoteIdentifier(r.contract.Columns.Site),
		quoteIdentifier(r.contract.Columns.SchemaName), quoteIdentifier(r.contract.Columns.DataJSON), r.submissionsTable,
		quoteIdentifier(r.contract.Columns.Site), quoteIdentifier(r.contract.Columns.SchemaName))
	arguments := []any{site, schema}
	if filter != nil {
		fieldJSON, err := json.Marshal(filter.Field)
		if err != nil {
			return nil, errors.New("encode submission filter")
		}
		dataColumn := quoteIdentifier(r.contract.Columns.DataJSON)
		query += fmt.Sprintf(" AND json_type(%s, ?) IS NOT NULL AND json_extract(%s, ?) IS json_extract(?, '$')", dataColumn, dataColumn)
		path := "$." + string(fieldJSON)
		arguments = append(arguments, path, path, string(filter.Equals))
	}
	query += fmt.Sprintf(" ORDER BY %s DESC, %s DESC LIMIT ?",
		quoteIdentifier(r.contract.Columns.CreatedAt), quoteIdentifier(r.contract.Columns.ID))
	arguments = append(arguments, limit)
	rows, err := r.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, errors.New("list submissions")
	}
	defer rows.Close()

	items := make([]models.Submission, 0, limit)
	for rows.Next() {
		var item models.Submission
		var rawData string
		if err := rows.Scan(&item.ID, &item.CreatedAt, &item.Site, &item.Schema, &rawData); err != nil {
			return nil, errors.New("read submission")
		}
		if err := json.Unmarshal([]byte(rawData), &item.Data); err != nil {
			return nil, errors.New("decode stored submission")
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.New("iterate submissions")
	}
	return items, nil
}

func (r *SQLiteRepository) Delete(ctx context.Context, site, schema, id string) error {
	result, err := r.db.ExecContext(ctx,
		fmt.Sprintf("DELETE FROM %s WHERE %s = ? AND %s = ? AND %s = ?", r.submissionsTable,
			quoteIdentifier(r.contract.Columns.Site), quoteIdentifier(r.contract.Columns.SchemaName), quoteIdentifier(r.contract.Columns.ID)),
		site, schema, id,
	)
	if err != nil {
		return errors.New("delete submission")
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return errors.New("check deleted submission")
	}
	if deleted == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLiteRepository) Close() error { return r.db.Close() }

func quoteIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

var _ interfaces.Repository = (*SQLiteRepository)(nil)
