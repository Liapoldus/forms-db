package sqlstore

// Durable names and row bounds preserve the existing storage/cursor contract.
const (
	defaultTablePrefix          string = "form_"
	initializationLockSuffix    string = "schema-init"
	schemasTableName            string = "schemas"
	submissionsTableName        string = "submissions"
	submissionScopeCreatedIndex string = "submissions_scope_created_idx"
	siteColumnName              string = "site"
	schemaNameColumnName        string = "schema_name"
	schemaJSONColumnName        string = "schema_json"
	updatedAtColumnName         string = "updated_at"
	idColumnName                string = "id"
	createdAtColumnName         string = "created_at"
	dataJSONColumnName          string = "data_json"
)

// Internal diagnostics deliberately exclude driver errors, SQL values and secrets.
const (
	messageInvalidFormsStorageConfiguration         string = "invalid forms storage configuration"
	messageLoadFormsStorageResourceLimits           string = "load forms storage resource limits"
	messageInvalidFormsStorageTablePrefix           string = "invalid forms storage table prefix"
	messageInvalidFormsSQLiteConnectionOptions      string = "invalid forms SQLite connection options"
	messageOpenFormsStorageRepository               string = "open forms storage repository"
	messageConnectFormsStorageRepository            string = "connect forms storage repository"
	messageFormsStorageOperationTimedOut            string = "forms storage operation timed out"
	messageOpenFormsStorageInitializationConnection string = "open forms storage initialization connection"
	messageUnlockFormsStorageInitialization         string = "unlock forms storage initialization"
	messageLockFormsStorageInitialization           string = "lock forms storage initialization"
	messageInitializeFormsSchemaTable               string = "initialize forms schema table"
	messageInitializeFormsSubmissionsTable          string = "initialize forms submissions table"
	messageInitializeFormsSubmissionsIndex          string = "initialize forms submissions index"
	messageEncodeSubmissionData                     string = "encode submission data"
	messageBeginSubmissionTransaction               string = "begin submission transaction"
	messageStoreSubmission                          string = "store submission"
	messageCommitSubmissionTransaction              string = "commit submission transaction"
	messagePersistConfiguredFormSchema              string = "persist configured form schema"
	messageListSubmissions                          string = "list submissions"
	messageReadSubmission                           string = "read submission"
	messageDecodeStoredSubmission                   string = "decode stored submission"
	messageIterateSubmissions                       string = "iterate submissions"
	messageDeleteSubmission                         string = "delete submission"
	messageCheckDeletedSubmission                   string = "check deleted submission"
)
