// Package sqlstore implements SQLite, PostgreSQL and MySQL form persistence.
package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"strings"
	"time"

	productcontracts "github.com/Liapoldus/forms-db/contracts"

	"github.com/Liapoldus/forms-db/internal/domain/interfaces"

	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type Repository struct {
	db                 *sql.DB
	dialect            sqlDialect
	initializationLock string
	schemasTable       string
	submissionsTable   string
	submissionIndex    string
	schemas            map[string]json.RawMessage
	operationSlots     chan struct{}
	operationTimeout   time.Duration
}

func NewRepository(ctx context.Context, driver, dsn, tablePrefix string, schemas map[string]json.RawMessage) (*Repository, error) {
	dialect, ok := sqlDialects()[driver]
	if !ok || strings.TrimSpace(dsn) == "" {
		return nil, errors.New(messageInvalidFormsStorageConfiguration)
	}
	if tablePrefix == "" {
		tablePrefix = defaultTablePrefix
	}
	limits, err := productcontracts.Limits()
	if err != nil {
		return nil, errors.New(messageLoadFormsStorageResourceLimits)
	}
	if !config.ValidTablePrefix(tablePrefix) {
		return nil, errors.New(messageInvalidFormsStorageTablePrefix)
	}
	if driver == "sqlite" {
		dsn, err = sqliteDSNWithBusyTimeout(dsn, limits)
		if err != nil {
			return nil, errors.New(messageInvalidFormsSQLiteConnectionOptions)
		}
	}
	db, err := sql.Open(dialect.name, dsn)
	if err != nil {
		return nil, errors.New(messageOpenFormsStorageRepository)
	}
	maxConnections := limits.DatabaseConcurrentOperationsMax
	if driver == "sqlite" {
		maxConnections = 1
	}
	db.SetMaxOpenConns(maxConnections)
	db.SetMaxIdleConns(maxConnections)
	operationTimeout := time.Duration(limits.DatabaseTimeoutMilliseconds) * time.Millisecond
	initCtx, cancelInit := context.WithTimeout(ctx, operationTimeout)
	defer cancelInit()
	if err := db.PingContext(initCtx); err != nil {
		_ = db.Close() //nolint:errcheck // Preserve the sanitized initialization failure; Close cannot restore a failed candidate.
		return nil, errors.New(messageConnectFormsStorageRepository)
	}

	repository := &Repository{
		db:                 db,
		dialect:            dialect,
		initializationLock: tablePrefix + initializationLockSuffix,
		schemasTable:       quoteIdentifier(dialect, tablePrefix+schemasTableName),
		submissionsTable:   quoteIdentifier(dialect, tablePrefix+submissionsTableName),
		submissionIndex:    quoteIdentifier(dialect, tablePrefix+submissionScopeCreatedIndex),
		schemas:            cloneSchemas(schemas),
		operationSlots:     make(chan struct{}, limits.DatabaseConcurrentOperationsMax),
		operationTimeout:   operationTimeout,
	}
	if err := repository.initializeWithLock(initCtx); err != nil {
		_ = db.Close() //nolint:errcheck // Preserve the sanitized initialization failure; Close cannot restore a failed candidate.
		return nil, err
	}
	return repository, nil
}

func (r *Repository) beginOperation(parent context.Context) (context.Context, func(), error) {
	ctx, cancel := context.WithTimeout(parent, r.operationTimeout)
	select {
	case r.operationSlots <- struct{}{}:
		return ctx, func() {
			<-r.operationSlots
			cancel()
		}, nil
	case <-ctx.Done():
		cancel()
		return nil, nil, errors.New(messageFormsStorageOperationTimedOut)
	}
}

func (r *Repository) Close() error { return r.db.Close() }

var _ interfaces.Repository = (*Repository)(nil)
