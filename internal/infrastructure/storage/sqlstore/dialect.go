package sqlstore

import (
	"fmt"

	"net/url"

	"strings"

	"github.com/Liapoldus/forms-db/contracts/policy"

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

func sqliteDSNWithBusyTimeout(dsn string, limits policy.ResourceLimits) (string, error) {
	base, query, hasQuery := strings.Cut(dsn, "?")
	values := make(url.Values)
	if hasQuery {
		parsed, err := url.ParseQuery(query)
		if err != nil {
			return "", err
		}
		values = parsed
	}
	pragmas := values[limits.SQLitePragmaQueryParameter]
	filtered := pragmas[:0]
	for _, pragma := range pragmas {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(pragma)), strings.ToLower(limits.SQLiteBusyTimeoutPragma)+"=") {
			filtered = append(filtered, pragma)
		}
	}
	filtered = append(filtered, fmt.Sprintf("%s=%d", limits.SQLiteBusyTimeoutPragma, limits.DatabaseTimeoutMilliseconds))
	values[limits.SQLitePragmaQueryParameter] = filtered
	return base + "?" + values.Encode(), nil
}

func sqlDialects() map[string]sqlDialect {
	return map[string]sqlDialect{
		"sqlite":   {name: "sqlite", quote: '`', jsonType: "TEXT", textType: "TEXT"},
		"postgres": {name: "pgx", quote: '"', jsonType: "JSONB", textType: "TEXT COLLATE \"C\""},
		"mysql":    {name: "mysql", quote: '`', jsonType: "JSON", textType: "VARCHAR(191) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin", indexInTable: true},
	}
}

func quoteIdentifier(dialect sqlDialect, value string) string {
	quote := string(dialect.quote)
	return quote + strings.ReplaceAll(value, quote, quote+quote) + quote
}

func (r *Repository) bind(index int) string {
	if r.dialect.name == "pgx" {
		return fmt.Sprintf(sqlPostgresBind, index)
	}
	return sqlPositionalBind
}
