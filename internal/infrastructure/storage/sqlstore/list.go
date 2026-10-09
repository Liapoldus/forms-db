package sqlstore

import (
	"context"

	"encoding/json"
	"errors"
	"fmt"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage/record"
	"math/big"

	"reflect"
	"strings"

	"github.com/Liapoldus/forms-db/internal/domain/models"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

func (r *Repository) List(ctx context.Context, site, schema string, filter *models.SubmissionFilter, after *models.SubmissionCursor, limit int) ([]models.Submission, error) {
	ctx, release, err := r.beginOperation(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if limit < 1 {
		limit = record.DefaultListRows
	}
	if limit > record.MaxListRows {
		limit = record.MaxListRows
	}
	idColumn := quoteIdentifier(r.dialect, idColumnName)
	createdAtColumn := quoteIdentifier(r.dialect, createdAtColumnName)
	siteColumn := quoteIdentifier(r.dialect, siteColumnName)
	schemaColumn := quoteIdentifier(r.dialect, schemaNameColumnName)
	dataColumn := quoteIdentifier(r.dialect, dataJSONColumnName)
	query := fmt.Sprintf(sqlListSubmissions,
		idColumn, createdAtColumn, siteColumn, schemaColumn, dataColumn, r.submissionsTable,
		siteColumn, r.bind(1), schemaColumn, r.bind(2),
	)
	arguments := []any{site, schema}
	if after != nil {
		query += fmt.Sprintf(sqlAfterCursor,
			createdAtColumn, r.bind(3), createdAtColumn, r.bind(4), idColumn, r.bind(5))
		arguments = append(arguments, after.CreatedAt, after.CreatedAt, after.ID)
	}
	query += fmt.Sprintf(sqlOrderSubmissions, createdAtColumn, idColumn)
	if filter == nil {
		query += sqlLimit + r.bind(len(arguments)+1)
		arguments = append(arguments, limit)
	}
	rows, err := r.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, errors.New(messageListSubmissions)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			// Query/iteration errors below preserve the public storage error mapping.
			return
		}
	}()
	items := make([]models.Submission, 0, limit)
	for rows.Next() {
		var item models.Submission
		var rawData string
		if err := rows.Scan(&item.ID, &item.CreatedAt, &item.Site, &item.Schema, &rawData); err != nil {
			return nil, errors.New(messageReadSubmission)
		}
		decoder := json.NewDecoder(strings.NewReader(rawData))
		decoder.UseNumber()
		if err := decoder.Decode(&item.Data); err != nil {
			return nil, errors.New(messageDecodeStoredSubmission)
		}
		if matchesJSONFilter(item.Data, filter) {
			items = append(items, item)
			if len(items) == limit {
				break
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, errors.New(messageIterateSubmissions)
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
