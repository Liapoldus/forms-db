package definition

func SettingsSchema() any {
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://liapoldus.github.io/plugins/forms-db/settings/v1/schema.json",
		"type":                 "object",
		"additionalProperties": false,
		"required": []any{
			"driver",
			"schemas",
		},
		"properties": map[string]any{
			"driver": map[string]any{
				"type": "string",
				"enum": []any{
					"sqlite",
					"mysql",
					"postgres",
					"memory",
				},
			},
			"dsn": map[string]any{
				"type":        "string",
				"minLength":   1,
				"maxLength":   1024,
				"description": "Opaque secret reference; never a connection string.",
			},
			"cursorSecretRef": map[string]any{
				"type":        "string",
				"minLength":   1,
				"maxLength":   1024,
				"description": "Opaque reference used only for per-call cursor signing grants.",
			},
			"tablePrefix": map[string]any{
				"type":    "string",
				"pattern": "^[A-Za-z_][A-Za-z0-9_]{0,31}$",
			},
			"schemas": map[string]any{
				"type": "object",
				"additionalProperties": map[string]any{
					"type": "object",
				},
			},
		},
		"allOf": []any{
			map[string]any{
				"if": map[string]any{
					"properties": map[string]any{
						"driver": map[string]any{
							"const": "memory",
						},
					},
					"required": []any{
						"driver",
					},
				},
				"then": map[string]any{
					"properties": map[string]any{
						"dsn": false,
					},
				},
				"else": map[string]any{
					"properties": map[string]any{
						"dsn": map[string]any{
							"type": "string",
						},
					},
					"required": []any{
						"dsn",
					},
				},
			},
		},
	}
}
