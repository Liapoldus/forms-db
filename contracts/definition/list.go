package definition

func ListErrors() any {
	return map[string]any{
		"$id":        "https://github.com/Liapoldus/forms-db/contracts/v1/list-errors.json",
		"capability": "forms.list",
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

func ListRequestSchema() any {
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://github.com/Liapoldus/forms-db/contracts/v1/list-request.schema.json",
		"title":                "Запрос forms.list v1",
		"description":          "Запрос страницы отправок. Cursor непрозрачен для вызывающей стороны; его формат и механизм защиты здесь не задаются.",
		"type":                 "object",
		"additionalProperties": false,
		"required": []any{
			"site",
			"schemaName",
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
			"cursor": map[string]any{
				"type":      "string",
				"maxLength": 4096,
			},
			"limit": map[string]any{
				"type":    "integer",
				"minimum": 1,
				"maximum": 100,
				"default": 50,
			},
			"filter": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required": []any{
					"field",
					"equals",
				},
				"properties": map[string]any{
					"field": map[string]any{
						"type":      "string",
						"minLength": 1,
					},
					"equals": true,
				},
			},
		},
	}
}

func ListResponseSchema() any {
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://github.com/Liapoldus/forms-db/contracts/v1/list-response.schema.json",
		"title":                "Ответ forms.list v1",
		"description":          "Страница отправок. Каждый элемент соответствует существующей сериализации Submission; nextCursor равен null, если следующей страницы нет.",
		"type":                 "object",
		"additionalProperties": false,
		"required": []any{
			"items",
			"nextCursor",
		},
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required": []any{
						"id",
						"createdAt",
						"site",
						"schemaName",
						"data",
					},
					"properties": map[string]any{
						"id": map[string]any{
							"type": "string",
						},
						"createdAt": map[string]any{
							"type": "string",
						},
						"site": map[string]any{
							"type": "string",
						},
						"schemaName": map[string]any{
							"type": "string",
						},
						"data": map[string]any{
							"type": "object",
						},
					},
				},
			},
			"nextCursor": map[string]any{
				"oneOf": []any{
					map[string]any{
						"type": "string",
					},
					map[string]any{
						"type": "null",
					},
				},
			},
		},
	}
}
