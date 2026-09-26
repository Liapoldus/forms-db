package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"time"

	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type sqlDialect struct {
	name         string
	quote        byte
	jsonType     string
	textType     string
	indexInTable bool
}

type SQLRepository struct {
	db               *sql.DB
	dialect          sqlDialect
	contract         storageContract
	schemasTable     string
	submissionsTable string
	submissionIndex  string
	schemas          map[string]json.RawMessage
}

func NewRepository(ctx context.Context, driver, dsn, tablePrefix string, schemas map[string]json.RawMessage) (*SQLRepository, error) {
	dialect, ok := sqlDialects()[driver]
	if !ok || strings.TrimSpace(dsn) == "" {
		return nil, errors.New("invalid forms storage configuration")
	}
	if tablePrefix == "" {
		tablePrefix = "form_"
	}
	contract, err := loadStorageContract()
	if err != nil {
		return nil, err
	}
	if !config.ValidTablePrefix(tablePrefix) {
		return nil, errors.New("invalid forms storage table prefix")
	}
	db, err := sql.Open(dialect.name, dsn)
	if err != nil {
		return nil, errors.New("open forms storage repository")
	}
	if driver == "sqlite" {
		db.SetMaxOpenConns(1)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, errors.New("connect forms storage repository")
	}

	repository := &SQLRepository{
		db:               db,
		dialect:          dialect,
		contract:         contract,
		schemasTable:     quoteIdentifier(dialect, tablePrefix+contract.Tables.Schemas),
		submissionsTable: quoteIdentifier(dialect, tablePrefix+contract.Tables.Submissions),
		submissionIndex:  quoteIdentifier(dialect, tablePrefix+contract.Indexes.SubmissionScopeCreatedAt),
		schemas:          cloneSchemas(schemas),
	}
	if err := repository.initialize(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return repository, nil
}

func sqlDialects() map[string]sqlDialect {
	return map[string]sqlDialect{
		"sqlite":   {name: "sqlite", quote: '`', jsonType: "TEXT", textType: "TEXT"},
		"postgres": {name: "pgx", quote: '"', jsonType: "JSONB", textType: "TEXT COLLATE \"C\""},
		"mysql":    {name: "mysql", quote: '`', jsonType: "JSON", textType: "VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin", indexInTable: true},
	}
}

func cloneSchemas(schemas map[string]json.RawMessage) map[string]json.RawMessage {
	copy := make(map[string]json.RawMessage, len(schemas))
	for name, schema := range schemas {
		copy[name] = append(json.RawMessage(nil), schema...)
	}
	return copy
}

func quoteIdentifier(dialect sqlDialect, value string) string {
	quote := string(dialect.quote)
	return quote + strings.ReplaceAll(value, quote, quote+quote) + quote
}

func (r *SQLRepository) bind(index int) string {
	if r.dialect.name == "pgx" {
		return fmt.Sprintf("$%d", index)
	}
	return "?"
}

func (r *SQLRepository) initialize(ctx context.Context) error {
	columns := r.contract.Columns
	statement := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (%s %s NOT NULL, %s %s NOT NULL, %s %s NOT NULL, %s %s NOT NULL, PRIMARY KEY (%s, %s))",
		r.schemasTable,
		quoteIdentifier(r.dialect, columns.Site), r.dialect.textType,
		quoteIdentifier(r.dialect, columns.SchemaName), r.dialect.textType,
		quoteIdentifier(r.dialect, columns.SchemaJSON), r.dialect.jsonType,
		quoteIdentifier(r.dialect, columns.UpdatedAt), r.dialect.textType,
		quoteIdentifier(r.dialect, columns.Site), quoteIdentifier(r.dialect, columns.SchemaName),
	)
	if _, err := r.db.ExecContext(ctx, statement); err != nil {
		return errors.New("initialize forms schema table")
	}

	idColumn := quoteIdentifier(r.dialect, columns.ID)
	siteColumn := quoteIdentifier(r.dialect, columns.Site)
	schemaColumn := quoteIdentifier(r.dialect, columns.SchemaName)
	createdAtColumn := quoteIdentifier(r.dialect, columns.CreatedAt)
	dataColumn := quoteIdentifier(r.dialect, columns.DataJSON)
	indexDDL := fmt.Sprintf("%s ON %s (%s, %s, %s, %s)", r.submissionIndex, r.submissionsTable, siteColumn, schemaColumn, createdAtColumn, idColumn)
	if r.dialect.indexInTable {
		statement = fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s (%s VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY, %s %s NOT NULL, %s %s NOT NULL, %s VARCHAR(40) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, %s %s NOT NULL, INDEX %s (%s, %s, %s, %s))",
			r.submissionsTable, idColumn,
			siteColumn, r.dialect.textType,
			schemaColumn, r.dialect.textType,
			createdAtColumn,
			dataColumn, r.dialect.jsonType,
			r.submissionIndex, siteColumn, schemaColumn, createdAtColumn, idColumn,
		)
	} else {
		statement = fmt.Sprintf(
			"CREATE TABLE IF NOT EXISTS %s (%s %s PRIMARY KEY, %s %s NOT NULL, %s %s NOT NULL, %s %s NOT NULL, %s %s NOT NULL)",
			r.submissionsTable, idColumn, r.dialect.textType,
			siteColumn, r.dialect.textType,
			schemaColumn, r.dialect.textType,
			createdAtColumn, r.dialect.textType,
			dataColumn, r.dialect.jsonType,
		)
	}
	if _, err := r.db.ExecContext(ctx, statement); err != nil {
		return errors.New("initialize forms submissions table")
	}
	if !r.dialect.indexInTable {
		statement = "CREATE INDEX IF NOT EXISTS " + indexDDL
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return errors.New("initialize forms submissions index")
		}
	}
	return nil
}

func (r *SQLRepository) Submit(ctx context.Context, submission models.Submission) (models.Submission, error) {
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
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return models.Submission{}, errors.New("begin submission transaction")
	}
	defer tx.Rollback()
	if schema, exists := r.schemas[submission.Schema]; exists {
		if err := r.upsertSchema(ctx, tx, submission.Site, submission.Schema, schema); err != nil {
			return models.Submission{}, err
		}
	}
	columns := r.contract.Columns
	query := fmt.Sprintf("INSERT INTO %s (%s, %s, %s, %s, %s) VALUES (%s, %s, %s, %s, %s)",
		r.submissionsTable,
		quoteIdentifier(r.dialect, columns.ID), quoteIdentifier(r.dialect, columns.Site),
		quoteIdentifier(r.dialect, columns.SchemaName), quoteIdentifier(r.dialect, columns.CreatedAt),
		quoteIdentifier(r.dialect, columns.DataJSON), r.bind(1), r.bind(2), r.bind(3), r.bind(4), r.bind(5),
	)
	if _, err := tx.ExecContext(ctx, query, submission.ID, submission.Site, submission.Schema, submission.CreatedAt, string(data)); err != nil {
		return models.Submission{}, errors.New("store submission")
	}
	if err := tx.Commit(); err != nil {
		return models.Submission{}, errors.New("commit submission transaction")
	}
	return submission, nil
}

func (r *SQLRepository) upsertSchema(ctx context.Context, tx *sql.Tx, site, name string, schema json.RawMessage) error {
	columns := r.contract.Columns
	siteColumn := quoteIdentifier(r.dialect, columns.Site)
	nameColumn := quoteIdentifier(r.dialect, columns.SchemaName)
	schemaColumn := quoteIdentifier(r.dialect, columns.SchemaJSON)
	updatedColumn := quoteIdentifier(r.dialect, columns.UpdatedAt)
	query := fmt.Sprintf("INSERT INTO %s (%s, %s, %s, %s) VALUES (%s, %s, %s, %s)",
		r.schemasTable, siteColumn, nameColumn, schemaColumn, updatedColumn,
		r.bind(1), r.bind(2), r.bind(3), r.bind(4),
	)
	switch r.dialect.name {
	case "pgx":
		query += fmt.Sprintf(" ON CONFLICT (%s, %s) DO UPDATE SET %s = EXCLUDED.%s, %s = EXCLUDED.%s", siteColumn, nameColumn, schemaColumn, schemaColumn, updatedColumn, updatedColumn)
	case "mysql":
		query += fmt.Sprintf(" ON DUPLICATE KEY UPDATE %s = VALUES(%s), %s = VALUES(%s)", schemaColumn, schemaColumn, updatedColumn, updatedColumn)
	default:
		query += fmt.Sprintf(" ON CONFLICT (%s, %s) DO UPDATE SET %s = excluded.%s, %s = excluded.%s", siteColumn, nameColumn, schemaColumn, schemaColumn, updatedColumn, updatedColumn)
	}
	if _, err := tx.ExecContext(ctx, query, site, name, string(schema), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return errors.New("persist configured form schema")
	}
	return nil
}

func (r *SQLRepository) List(ctx context.Context, site, schema string, filter *models.SubmissionFilter, after *models.SubmissionCursor, limit int) ([]models.Submission, error) {
	if limit < 1 {
		limit = 50
	}
	if limit > r.contract.MaxListRows {
		limit = r.contract.MaxListRows
	}
	columns := r.contract.Columns
	idColumn := quoteIdentifier(r.dialect, columns.ID)
	createdAtColumn := quoteIdentifier(r.dialect, columns.CreatedAt)
	siteColumn := quoteIdentifier(r.dialect, columns.Site)
	schemaColumn := quoteIdentifier(r.dialect, columns.SchemaName)
	dataColumn := quoteIdentifier(r.dialect, columns.DataJSON)
	query := fmt.Sprintf("SELECT %s, %s, %s, %s, %s FROM %s WHERE %s = %s AND %s = %s",
		idColumn, createdAtColumn, siteColumn, schemaColumn, dataColumn, r.submissionsTable,
		siteColumn, r.bind(1), schemaColumn, r.bind(2),
	)
	arguments := []any{site, schema}
	if after != nil {
		query += fmt.Sprintf(" AND (%s < %s OR (%s = %s AND %s < %s))",
			createdAtColumn, r.bind(3), createdAtColumn, r.bind(4), idColumn, r.bind(5))
		arguments = append(arguments, after.CreatedAt, after.CreatedAt, after.ID)
	}
	query += fmt.Sprintf(" ORDER BY %s DESC, %s DESC", createdAtColumn, idColumn)
	if filter == nil {
		query += " LIMIT " + r.bind(len(arguments)+1)
		arguments = append(arguments, limit)
	}
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
		decoder := json.NewDecoder(strings.NewReader(rawData))
		decoder.UseNumber()
		if err := decoder.Decode(&item.Data); err != nil {
			return nil, errors.New("decode stored submission")
		}
		if matchesJSONFilter(item.Data, filter) {
			items = append(items, item)
			if len(items) == limit {
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, errors.New("iterate submissions")
	}
	return items, nil
}

func matchesJSONFilter(data map[string]any, filter *models.SubmissionFilter) bool {
	if filter == nil {
		return true
	}
	actual, exists := data[filter.Field]
	if !exists {
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(string(filter.Equals)))
	decoder.UseNumber()
	var expected any
	if err := decoder.Decode(&expected); err != nil {
		return false
	}
	return equalJSON(actual, expected)
}

func equalJSON(left, right any) bool {
	switch leftValue := left.(type) {
	case json.Number:
		rightValue, ok := right.(json.Number)
		if !ok {
			return false
		}
		leftNumber, leftOK := new(big.Rat).SetString(string(leftValue))
		rightNumber, rightOK := new(big.Rat).SetString(string(rightValue))
		return leftOK && rightOK && leftNumber.Cmp(rightNumber) == 0
	case map[string]any:
		rightValue, ok := right.(map[string]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for key, value := range leftValue {
			other, exists := rightValue[key]
			if !exists || !equalJSON(value, other) {
				return false
			}
		}
		return true
	case []any:
		rightValue, ok := right.([]any)
		if !ok || len(leftValue) != len(rightValue) {
			return false
		}
		for index, value := range leftValue {
			if !equalJSON(value, rightValue[index]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(left, right)
	}
}

func (r *SQLRepository) Delete(ctx context.Context, site, schema, id string) error {
	columns := r.contract.Columns
	query := fmt.Sprintf("DELETE FROM %s WHERE %s = %s AND %s = %s AND %s = %s",
		r.submissionsTable,
		quoteIdentifier(r.dialect, columns.Site), r.bind(1),
		quoteIdentifier(r.dialect, columns.SchemaName), r.bind(2),
		quoteIdentifier(r.dialect, columns.ID), r.bind(3),
	)
	result, err := r.db.ExecContext(ctx, query, site, schema, id)
	if err != nil {
		return errors.New("delete submission")
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return errors.New("check deleted submission")
	}
	if deleted == 0 {
		return interfaces.ErrNotFound
	}
	return nil
}

func (r *SQLRepository) Close() error { return r.db.Close() }

var _ interfaces.Repository = (*SQLRepository)(nil)
