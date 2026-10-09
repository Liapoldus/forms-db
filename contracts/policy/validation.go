package policy

type ValidationProfile struct {
	SchemaNamePattern       string `json:"schemaNamePattern"`
	TablePrefixPattern      string `json:"tablePrefixPattern"`
	Draft2020SchemaURL      string `json:"draft2020SchemaUrl"`
	ResourceBase            string `json:"resourceBase"`
	MaxSubmissionProperties int    `json:"maxSubmissionProperties"`
}

func Validation() ValidationProfile {
	return ValidationProfile{
		SchemaNamePattern:       "^[a-z][a-z0-9_-]{0,63}$",
		TablePrefixPattern:      "^[A-Za-z_][A-Za-z0-9_]{0,31}$",
		Draft2020SchemaURL:      "https://json-schema.org/draft/2020-12/schema",
		ResourceBase:            "https://schemas.invalid/liapoldus/forms-db/",
		MaxSubmissionProperties: 256,
	}
}
