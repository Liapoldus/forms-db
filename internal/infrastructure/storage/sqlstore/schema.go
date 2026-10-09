package sqlstore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"

	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

func cloneSchemas(schemas map[string]json.RawMessage) map[string]json.RawMessage {
	copy := make(map[string]json.RawMessage, len(schemas))
	for name, schema := range schemas {
		copy[name] = append(json.RawMessage(nil), schema...)
	}
	return copy
}

type sqlExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (r *Repository) initializeWithLock(ctx context.Context) error {
	connection, err := r.db.Conn(ctx)
	if err != nil {
		return errors.New(messageOpenFormsStorageInitializationConnection)
	}
	defer func() {
		if err := connection.Close(); err != nil {
			// database/sql has already discarded a failed or unlocked connection.
			return
		}
	}()

	release, err := r.acquireInitializationLock(ctx, connection)
	if err != nil {
		return err
	}
	initializeErr := r.initialize(ctx, connection)
	releaseErr := release()
	if releaseErr != nil {
		if err := connection.Raw(func(any) error { return driver.ErrBadConn }); !errors.Is(err, driver.ErrBadConn) {
			return errors.New(messageUnlockFormsStorageInitialization)
		}
		if initializeErr != nil {
			return initializeErr
		}
		return errors.New(messageUnlockFormsStorageInitialization)
	}
	return initializeErr
}

func (r *Repository) acquireInitializationLock(ctx context.Context, connection *sql.Conn) (func() error, error) {
	switch r.dialect.name {
	case "pgx":
		if _, err := connection.ExecContext(ctx, sqlPostgresLock, r.initializationLock); err != nil {
			return nil, errors.New(messageLockFormsStorageInitialization)
		}
		return func() error {
			releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.operationTimeout)
			defer cancel()
			var released bool
			if err := connection.QueryRowContext(releaseCtx, sqlPostgresUnlock, r.initializationLock).Scan(&released); err != nil || !released {
				return errors.New(messageUnlockFormsStorageInitialization)
			}
			return nil
		}, nil
	case "mysql":
		var acquired sql.NullInt64
		waitSeconds := int64(r.operationTimeout / time.Second)
		if waitSeconds < 1 {
			waitSeconds = 1
		}
		if err := connection.QueryRowContext(ctx, sqlMySQLLock, r.initializationLock, waitSeconds).Scan(&acquired); err != nil || !acquired.Valid || acquired.Int64 != 1 {
			return nil, errors.New(messageLockFormsStorageInitialization)
		}
		return func() error {
			releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.operationTimeout)
			defer cancel()
			var released sql.NullInt64
			if err := connection.QueryRowContext(releaseCtx, sqlMySQLUnlock, r.initializationLock).Scan(&released); err != nil || !released.Valid || released.Int64 != 1 {
				return errors.New(messageUnlockFormsStorageInitialization)
			}
			return nil
		}, nil
	default:
		return func() error { return nil }, nil
	}
}

func (r *Repository) initialize(ctx context.Context, connection sqlExecer) error {
	statement := fmt.Sprintf(
		sqlCreateSchemas,
		r.schemasTable,
		quoteIdentifier(r.dialect, siteColumnName), r.dialect.textType,
		quoteIdentifier(r.dialect, schemaNameColumnName), r.dialect.textType,
		quoteIdentifier(r.dialect, schemaJSONColumnName), r.dialect.jsonType,
		quoteIdentifier(r.dialect, updatedAtColumnName), r.dialect.textType,
		quoteIdentifier(r.dialect, siteColumnName), quoteIdentifier(r.dialect, schemaNameColumnName),
	)
	if _, err := connection.ExecContext(ctx, statement); err != nil {
		return errors.New(messageInitializeFormsSchemaTable)
	}

	idColumn := quoteIdentifier(r.dialect, idColumnName)
	siteColumn := quoteIdentifier(r.dialect, siteColumnName)
	schemaColumn := quoteIdentifier(r.dialect, schemaNameColumnName)
	createdAtColumn := quoteIdentifier(r.dialect, createdAtColumnName)
	dataColumn := quoteIdentifier(r.dialect, dataJSONColumnName)
	indexDDL := fmt.Sprintf(sqlScopeIndex, r.submissionIndex, r.submissionsTable, siteColumn, schemaColumn, createdAtColumn, idColumn)
	if r.dialect.indexInTable {
		statement = fmt.Sprintf(
			sqlCreateMySQLSubmissions,
			r.submissionsTable, idColumn,
			siteColumn, r.dialect.textType,
			schemaColumn, r.dialect.textType,
			createdAtColumn,
			dataColumn, r.dialect.jsonType,
			r.submissionIndex, siteColumn, schemaColumn, createdAtColumn, idColumn,
		)
	} else {
		statement = fmt.Sprintf(
			sqlCreateSubmissions,
			r.submissionsTable, idColumn, r.dialect.textType,
			siteColumn, r.dialect.textType,
			schemaColumn, r.dialect.textType,
			createdAtColumn, r.dialect.textType,
			dataColumn, r.dialect.jsonType,
		)
	}
	if _, err := connection.ExecContext(ctx, statement); err != nil {
		return errors.New(messageInitializeFormsSubmissionsTable)
	}
	if !r.dialect.indexInTable {
		statement = sqlCreateIndex + indexDDL
		if _, err := connection.ExecContext(ctx, statement); err != nil {
			return errors.New(messageInitializeFormsSubmissionsIndex)
		}
	}
	return nil
}

func (r *Repository) upsertSchema(ctx context.Context, tx *sql.Tx, site, name string, schema json.RawMessage) error {
	siteColumn := quoteIdentifier(r.dialect, siteColumnName)
	nameColumn := quoteIdentifier(r.dialect, schemaNameColumnName)
	schemaColumn := quoteIdentifier(r.dialect, schemaJSONColumnName)
	updatedColumn := quoteIdentifier(r.dialect, updatedAtColumnName)
	query := fmt.Sprintf(sqlInsertSchema,
		r.schemasTable, siteColumn, nameColumn, schemaColumn, updatedColumn,
		r.bind(1), r.bind(2), r.bind(3), r.bind(4),
	)
	switch r.dialect.name {
	case "pgx":
		query += fmt.Sprintf(sqlPostgresUpsert, siteColumn, nameColumn, schemaColumn, schemaColumn, updatedColumn, updatedColumn)
	case "mysql":
		query += fmt.Sprintf(sqlMySQLUpsert, schemaColumn, schemaColumn, updatedColumn, updatedColumn)
	default:
		query += fmt.Sprintf(sqlSQLiteUpsert, siteColumn, nameColumn, schemaColumn, schemaColumn, updatedColumn, updatedColumn)
	}
	if _, err := tx.ExecContext(ctx, query, site, name, string(schema), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return errors.New(messagePersistConfiguredFormSchema)
	}
	return nil
}
