package definition

func SubmitErrors() any {
	return map[string]any{
		"$id":        "https://github.com/Liapoldus/forms-db/contracts/v1/submit-errors.json",
		"capability": "forms.submit",
		"version":    1,
		"errors": map[string]any{
			"validation_failed": map[string]any{
				"http":      422,
				"retryable": false,
			},
			"storage_unavailable": map[string]any{
				"http":      503,
				"retryable": true,
			},
		},
	}
}

func SubmitNegativeVectors() any {
	return map[string]any{
		"$id":     "https://github.com/Liapoldus/forms-db/contracts/v1/submit-negative-vectors.json",
		"version": 1,
		"schema":  "contracts/v1/submit-request.schema.json",
		"cases": []any{
			map[string]any{
				"name": "missing-data",
				"payload": map[string]any{
					"site":       "portal",
					"schemaName": "contact",
				},
				"expectedStatus": 422,
				"expectedCode":   "validation_failed",
			},
			map[string]any{
				"name": "non-object-data",
				"payload": map[string]any{
					"site":       "portal",
					"schemaName": "contact",
					"data": []any{
						"not",
						"an",
						"object",
					},
				},
				"expectedStatus": 422,
				"expectedCode":   "validation_failed",
			},
			map[string]any{
				"name": "additional-request-property",
				"payload": map[string]any{
					"site":       "portal",
					"schemaName": "contact",
					"data":       map[string]any{},
					"unexpected": true,
				},
				"expectedStatus": 422,
				"expectedCode":   "validation_failed",
			},
			map[string]any{
				"name": "configured-schema-rejects-value",
				"payload": map[string]any{
					"site":       "portal",
					"schemaName": "contact",
					"data": map[string]any{
						"email": 7,
					},
				},
				"expectedStatus": 422,
				"expectedCode":   "validation_failed",
			},
		},
	}
}

func SubmitRequestSchema() any {
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://github.com/Liapoldus/forms-db/contracts/v1/submit-request.schema.json",
		"title":                "Запрос forms.submit v1",
		"description":          "Продуктовый peer payload. Содержимое data дополнительно проверяется по JSON Schema активной конфигурации forms-db.",
		"type":                 "object",
		"additionalProperties": false,
		"required": []any{
			"site",
			"schemaName",
			"data",
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
			"data": map[string]any{
				"type": "object",
			},
		},
	}
}

func SubmitResponseSchema() any {
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://github.com/Liapoldus/forms-db/contracts/v1/submit-response.schema.json",
		"title":                "Ответ forms.submit v1",
		"type":                 "object",
		"additionalProperties": false,
		"required": []any{
			"id",
			"createdAt",
			"data",
		},
		"properties": map[string]any{
			"id": map[string]any{
				"type":    "string",
				"pattern": "^frm_[A-Za-z0-9_-]{24}$",
			},
			"createdAt": map[string]any{
				"type":   "string",
				"format": "date-time",
			},
			"data": map[string]any{
				"type": "object",
			},
		},
	}
}
