package storage

import (
	"context"
	"encoding/json"
)

type SQLiteRepository = SQLRepository

func NewSQLiteRepository(ctx context.Context, dsn, tablePrefix string) (*SQLiteRepository, error) {
	return NewRepository(ctx, "sqlite", dsn, tablePrefix, map[string]json.RawMessage{})
}
