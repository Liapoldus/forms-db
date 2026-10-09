package policy

type ResourceLimits struct {
	Version                         int    `json:"version"`
	SettingsMaxBytes                int    `json:"settingsMaxBytes"`
	SchemaMaxDepth                  int    `json:"schemaMaxDepth"`
	FieldsPerFormMax                int    `json:"fieldsPerFormMax"`
	SubmissionRequestMaxBytes       int    `json:"submissionRequestMaxBytes"`
	DatabaseTimeoutMilliseconds     int    `json:"databaseTimeoutMilliseconds"`
	DatabaseConcurrentOperationsMax int    `json:"databaseConcurrentOperationsMax"`
	SQLitePragmaQueryParameter      string `json:"sqlitePragmaQueryParameter"`
	SQLiteBusyTimeoutPragma         string `json:"sqliteBusyTimeoutPragma"`
	PageSizeMaximum                 int    `json:"pageSizeMaximum"`
	CursorMaximumBytes              int    `json:"cursorMaximumBytes"`
}

func Limits() ResourceLimits {
	return ResourceLimits{
		Version:                         1,
		SettingsMaxBytes:                262144,
		SchemaMaxDepth:                  32,
		FieldsPerFormMax:                128,
		SubmissionRequestMaxBytes:       1048576,
		DatabaseTimeoutMilliseconds:     5000,
		DatabaseConcurrentOperationsMax: 64,
		SQLitePragmaQueryParameter:      "_pragma",
		SQLiteBusyTimeoutPragma:         "busy_timeout",
		PageSizeMaximum:                 100,
		CursorMaximumBytes:              4096,
	}
}
