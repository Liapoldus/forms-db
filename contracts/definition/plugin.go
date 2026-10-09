package definition

func Plugin() any {
	return map[string]any{
		"name": "forms-db",
		"configuration": map[string]any{
			"schemaVersion": 1,
			"schema":        "contracts/v1/settings.schema.json",
		},
		"capabilities": []any{
			map[string]any{
				"name": "forms.submit",
				"mode": "call",
			},
			map[string]any{
				"name": "forms.list",
				"mode": "call",
			},
			map[string]any{
				"name": "forms.delete",
				"mode": "call",
			},
		},
		"adminSurface": "contracts/v1/admin-surface.json",
	}
}
