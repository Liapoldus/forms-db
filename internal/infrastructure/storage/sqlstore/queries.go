package sqlstore

// SQL templates are code-owned. Only validated, quoted identifiers and
// dialect placeholders are formatted into them; values remain bound arguments.
const (
	sqlPostgresLock           string = "SELECT pg_advisory_lock(hashtext($1)::bigint)"
	sqlPostgresUnlock         string = "SELECT pg_advisory_unlock(hashtext($1)::bigint)"
	sqlMySQLLock              string = "SELECT GET_LOCK(?, ?)"
	sqlMySQLUnlock            string = "SELECT RELEASE_LOCK(?)"
	sqlCreateSchemas          string = "CREATE TABLE IF NOT EXISTS %s (%s %s NOT NULL, %s %s NOT NULL, %s %s NOT NULL, %s %s NOT NULL, PRIMARY KEY (%s, %s))"
	sqlScopeIndex             string = "%s ON %s (%s, %s, %s, %s)"
	sqlCreateMySQLSubmissions string = "CREATE TABLE IF NOT EXISTS %s (%s VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY, %s %s NOT NULL, %s %s NOT NULL, %s VARCHAR(40) CHARACTER SET ascii COLLATE ascii_bin NOT NULL, %s %s NOT NULL, INDEX %s (%s, %s, %s, %s))"
	sqlCreateSubmissions      string = "CREATE TABLE IF NOT EXISTS %s (%s %s PRIMARY KEY, %s %s NOT NULL, %s %s NOT NULL, %s %s NOT NULL, %s %s NOT NULL)"
	sqlCreateIndex            string = "CREATE INDEX IF NOT EXISTS "
	sqlInsertSubmission       string = "INSERT INTO %s (%s, %s, %s, %s, %s) VALUES (%s, %s, %s, %s, %s)"
	sqlInsertSchema           string = "INSERT INTO %s (%s, %s, %s, %s) VALUES (%s, %s, %s, %s)"
	sqlPostgresUpsert         string = " ON CONFLICT (%s, %s) DO UPDATE SET %s = EXCLUDED.%s, %s = EXCLUDED.%s"
	sqlMySQLUpsert            string = " ON DUPLICATE KEY UPDATE %s = VALUES(%s), %s = VALUES(%s)"
	sqlSQLiteUpsert           string = " ON CONFLICT (%s, %s) DO UPDATE SET %s = excluded.%s, %s = excluded.%s"
	sqlListSubmissions        string = "SELECT %s, %s, %s, %s, %s FROM %s WHERE %s = %s AND %s = %s"
	sqlAfterCursor            string = " AND (%s < %s OR (%s = %s AND %s < %s))"
	sqlOrderSubmissions       string = " ORDER BY %s DESC, %s DESC"
	sqlLimit                  string = " LIMIT "
	sqlDeleteSubmission       string = "DELETE FROM %s WHERE %s = %s AND %s = %s AND %s = %s"
	sqlPostgresBind           string = "$%d"
	sqlPositionalBind         string = "?"
)
