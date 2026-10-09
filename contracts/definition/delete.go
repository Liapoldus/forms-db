package definition

func DeleteErrors() any {
	return map[string]any{
		"$id":           "https://github.com/Liapoldus/forms-db/contracts/v1/delete-errors.json",
		"capability":    "forms.delete",
		"version":       1,
		"missingRecord": "not_found",
		"semantics":     "Удаление отсутствующей записи возвращает not_found (HTTP 404); ошибка считается конечной и не повторяется автоматически.",
		"errors": map[string]any{
			"validation_failed": map[string]any{
				"http":      422,
				"retryable": false,
			},
			"not_found": map[string]any{
				"http":      404,
				"retryable": false,
			},
			"storage_unavailable": map[string]any{
				"http":      503,
				"retryable": true,
			},
		},
	}
}

func DeleteNegativeVectors() any {
	return []any{
		map[string]any{
			"name":   "forms-delete-missing-id",
			"schema": "contracts/v1/delete-request.schema.json",
			"payload": map[string]any{
				"site":       "portal",
				"schemaName": "contact",
			},
		},
		map[string]any{
			"name":   "forms-delete-empty-site",
			"schema": "contracts/v1/delete-request.schema.json",
			"payload": map[string]any{
				"site":       "",
				"schemaName": "contact",
				"id":         "frm_123",
			},
		},
		map[string]any{
			"name":   "forms-delete-unknown-field",
			"schema": "contracts/v1/delete-request.schema.json",
			"payload": map[string]any{
				"site":       "portal",
				"schemaName": "contact",
				"id":         "frm_123",
				"force":      true,
			},
		},
	}
}

func DeleteRequestSchema() any {
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://github.com/Liapoldus/forms-db/contracts/v1/delete-request.schema.json",
		"title":                "Запрос forms.delete v1",
		"description":          "Удаляет одну отправку формы из точного scope site/schemaName.",
		"type":                 "object",
		"additionalProperties": false,
		"required": []any{
			"site",
			"schemaName",
			"id",
		},
		"properties": map[string]any{
			"site": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"schemaName": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"id": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
		},
	}
}

func DeleteResponseSchema() any {
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://github.com/Liapoldus/forms-db/contracts/v1/delete-response.schema.json",
		"title":                "Ответ forms.delete v1",
		"type":                 "object",
		"additionalProperties": false,
		"required": []any{
			"deleted",
			"id",
		},
		"properties": map[string]any{
			"deleted": map[string]any{
				"const": true,
			},
			"id": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
		},
	}
}
