package definition

func CursorReplicaVectors() any {
	return map[string]any{
		"version": 1,
		"semantics": map[string]any{
			"keyDistribution":         "Every replica receives the same current Core-managed logical key in its per-call forms.list grant.",
			"rotation":                "Core replaces the logical key for subsequent grants; the plugin keeps no previous-key fallback, so cursors issued with the old key are rejected after rotation.",
			"replicaFailure":          "Failure to redeem a cursor grant fails that forms.list call closed as storage_unavailable and does not affect another replica.",
			"settingsRevisionBinding": "Cursor claims are bound to site, schemaName, and equality filter, not to a settings revision; cursor signing keys are retrieved per call and are not included in plugin settings.",
			"replay":                  "A valid, unexpired cursor is a reusable read-only continuation token. Repeating it with the same scope repeats the page query; it is not a one-use grant. Every invocation still obtains its own scoped signing-key grant.",
			"tokenVersion":            "A token version other than the configured version is rejected as validation_failed.",
		},
		"vectors": []any{
			map[string]any{
				"name":           "shared-current-key-cross-replica",
				"expectedStatus": 200,
			},
			map[string]any{
				"name":           "valid-cursor-replay",
				"expectedStatus": 200,
			},
			map[string]any{
				"name":           "old-cursor-after-key-rotation",
				"expectedStatus": 422,
			},
			map[string]any{
				"name":           "cursor-grant-unavailable-on-one-replica",
				"expectedStatus": 503,
			},
			map[string]any{
				"name":           "unsupported-token-version",
				"expectedStatus": 422,
			},
		},
	}
}

func RequestJSONVectors() any {
	return map[string]any{
		"$id":         "https://github.com/Liapoldus/forms-db/contracts/v1/request-json-vectors.json",
		"version":     1,
		"description": "Каждый capability request содержит один JSON object. Дублирующиеся JSON members отклоняются; имена верхнего уровня, сопоставляемые Go-структурам, также не могут различаться только регистром. Второй JSON document после первого запрещён.",
		"invalidRequests": []any{
			map[string]any{
				"name":           "duplicate-root-member",
				"capability":     "forms.submit",
				"payload":        "{\"site\":\"portal\",\"site\":\"attacker\",\"schemaName\":\"contact\",\"data\":{\"email\":\"duplicate@example.test\"}}",
				"expectedStatus": 422,
				"expectedCode":   "validation_failed",
			},
			map[string]any{
				"name":           "case-folded-root-member-collision",
				"capability":     "forms.submit",
				"payload":        "{\"site\":\"portal\",\"SITE\":\"attacker\",\"schemaName\":\"contact\",\"data\":{\"email\":\"case-folded@example.test\"}}",
				"expectedStatus": 422,
				"expectedCode":   "validation_failed",
			},
			map[string]any{
				"name":           "duplicate-nested-member",
				"capability":     "forms.submit",
				"payload":        "{\"site\":\"portal\",\"schemaName\":\"contact\",\"data\":{\"email\":\"first@example.test\",\"email\":\"second@example.test\"}}",
				"expectedStatus": 422,
				"expectedCode":   "validation_failed",
			},
			map[string]any{
				"name":           "trailing-json-document",
				"capability":     "forms.submit",
				"payload":        "{\"site\":\"portal\",\"schemaName\":\"contact\",\"data\":{\"email\":\"trailing@example.test\"}} {}",
				"expectedStatus": 422,
				"expectedCode":   "validation_failed",
			},
			map[string]any{
				"name":           "list-duplicate-root-member",
				"capability":     "forms.list",
				"payload":        "{\"site\":\"portal\",\"site\":\"attacker\",\"schemaName\":\"contact\"}",
				"expectedStatus": 422,
				"expectedCode":   "validation_failed",
			},
			map[string]any{
				"name":           "delete-trailing-json-document",
				"capability":     "forms.delete",
				"payload":        "{\"site\":\"portal\",\"schemaName\":\"contact\",\"id\":\"frm_fixture\"} {}",
				"expectedStatus": 422,
				"expectedCode":   "validation_failed",
			},
		},
	}
}
