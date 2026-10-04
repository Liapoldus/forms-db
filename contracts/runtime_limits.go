package contracts

import (
	"encoding/json"
	"errors"
	"sync"
)

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

var (
	resourceLimitsOnce sync.Once
	resourceLimits     ResourceLimits
	resourceLimitsErr  error
)

func Limits() (ResourceLimits, error) {
	resourceLimitsOnce.Do(func() {
		contents, err := pluginDocument("v1/runtime-limits.json")
		if err != nil || json.Unmarshal(contents, &resourceLimits) != nil {
			resourceLimitsErr = ErrInvalidPluginContract
			return
		}
		if resourceLimits.Version != 1 || resourceLimits.SettingsMaxBytes < 1 || resourceLimits.SchemaMaxDepth < 1 ||
			resourceLimits.FieldsPerFormMax < 1 || resourceLimits.SubmissionRequestMaxBytes < 1 ||
			resourceLimits.DatabaseTimeoutMilliseconds < 1 || resourceLimits.DatabaseConcurrentOperationsMax < 1 ||
			resourceLimits.SQLitePragmaQueryParameter == "" || resourceLimits.SQLiteBusyTimeoutPragma == "" ||
			resourceLimits.PageSizeMaximum < 1 || resourceLimits.CursorMaximumBytes < 1 {
			resourceLimitsErr = errors.New("incomplete forms-db resource limit contract")
		}
	})
	if resourceLimitsErr != nil {
		return ResourceLimits{}, resourceLimitsErr
	}
	return resourceLimits, nil
}
