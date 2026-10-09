package definition

func AdminSurfaceRequestSchema() any {
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://github.com/Liapoldus/forms-db/contracts/v1/admin-surface-request.schema.json",
		"title":                "Запрос поверхности администратора forms-db v1",
		"type":                 "object",
		"additionalProperties": false,
		"required": []any{
			"version",
		},
		"properties": map[string]any{
			"version": map[string]any{
				"const": 1,
			},
		},
	}
}

func AdminSurfaceResponseSchema() any {
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://github.com/Liapoldus/forms-db/contracts/v1/admin-surface-response.schema.json",
		"title":                "Ответ поверхности администратора forms-db v1",
		"type":                 "object",
		"additionalProperties": true,
		"required": []any{
			"version",
			"plugin",
			"requiredCapabilities",
		},
		"properties": map[string]any{
			"version": map[string]any{
				"const": 1,
			},
			"plugin": map[string]any{
				"type":      "string",
				"minLength": 1,
			},
			"requiredCapabilities": map[string]any{
				"type":        "array",
				"minItems":    1,
				"uniqueItems": true,
				"items": map[string]any{
					"type":      "string",
					"minLength": 1,
				},
			},
		},
	}
}

func AdminSurfaceVectors() any {
	return []any{
		map[string]any{
			"name": "forms-delete-from-selected-submission-row",
			"selectedRow": map[string]any{
				"id":         "frm_123",
				"site":       "portal",
				"schemaName": "contact",
				"createdAt":  "2026-01-01T00:00:00Z",
				"data": map[string]any{
					"email": "a@example.com",
				},
			},
			"expectedPayload": map[string]any{
				"site":       "portal",
				"schemaName": "contact",
				"id":         "frm_123",
			},
		},
	}
}

func AdminSurface() any {
	return map[string]any{
		"version": 1,
		"plugin":  "forms-db",
		"requiredCapabilities": []any{
			"forms.list",
			"forms.delete",
		},
		"pages": []any{
			map[string]any{
				"id":         "submissions",
				"title":      "Form submissions",
				"capability": "forms.list",
				"query": map[string]any{
					"id":         "query",
					"capability": "forms.list",
					"inputSchema": map[string]any{
						"type": "object",
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
								"type": "object",
								"properties": map[string]any{
									"field": map[string]any{
										"type":      "string",
										"minLength": 1,
									},
									"equals": true,
								},
								"required": []any{
									"field",
									"equals",
								},
								"additionalProperties": false,
							},
						},
						"required": []any{
							"site",
							"schemaName",
						},
						"additionalProperties": false,
					},
				},
				"permissions": []any{
					"plugins.forms-db.read",
					"plugins.forms-db.write",
				},
				"sections": []any{
					map[string]any{
						"id":   "filters",
						"kind": "form",
						"fields": []any{
							map[string]any{
								"key":      "site",
								"type":     "select",
								"required": true,
							},
							map[string]any{
								"key":      "schemaName",
								"type":     "select",
								"required": true,
							},
							map[string]any{
								"key":  "field",
								"type": "string",
							},
							map[string]any{
								"key":  "equals",
								"type": "string",
							},
						},
					},
					map[string]any{
						"id":             "records",
						"kind":           "table",
						"dataCapability": "forms.list",
						"columns": []any{
							"id",
							"site",
							"schemaName",
							"createdAt",
							"data",
						},
						"actions": []any{
							map[string]any{
								"id":           "delete",
								"title":        "Delete submission",
								"capability":   "forms.delete",
								"confirmation": "Delete this submission permanently?",
								"dangerous":    true,
								"inputSchema": map[string]any{
									"type": "object",
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
									"required": []any{
										"site",
										"schemaName",
										"id",
									},
									"additionalProperties": false,
								},
								"rowInput": map[string]any{
									"site":       "site",
									"schemaName": "schemaName",
									"id":         "id",
								},
							},
						},
					},
				},
			},
		},
	}
}
