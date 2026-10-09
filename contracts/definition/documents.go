// Package definition owns public schemas, error catalogs and contract vectors.
package definition

import "github.com/Liapoldus/forms-db/contracts/policy"

// Documents returns fresh code-owned contract definitions for each caller.
func Documents() map[string]any {
	return map[string]any{
		"v1/admin-surface-request.schema.json":  AdminSurfaceRequestSchema(),
		"v1/admin-surface-response.schema.json": AdminSurfaceResponseSchema(),
		"v1/admin-surface-vectors.json":         AdminSurfaceVectors(),
		"v1/admin-surface.json":                 AdminSurface(),
		"v1/cursor-replica-vectors.json":        CursorReplicaVectors(),
		"v1/delete-errors.json":                 DeleteErrors(),
		"v1/delete-negative-vectors.json":       DeleteNegativeVectors(),
		"v1/delete-request.schema.json":         DeleteRequestSchema(),
		"v1/delete-response.schema.json":        DeleteResponseSchema(),
		"v1/list-errors.json":                   ListErrors(),
		"v1/list-request.schema.json":           ListRequestSchema(),
		"v1/list-response.schema.json":          ListResponseSchema(),
		"v1/plugin.json":                        Plugin(),
		"v1/request-json-vectors.json":          RequestJSONVectors(),
		"v1/runtime-limits.json":                RuntimeLimits(),
		"v1/cursor.json":                        policy.Cursor(),
		"v1/secrets.json":                       policy.Secrets(),
		"v1/validation.json":                    policy.Validation(),
		"v1/settings.schema.json":               SettingsSchema(),
		"v1/submit-errors.json":                 SubmitErrors(),
		"v1/submit-negative-vectors.json":       SubmitNegativeVectors(),
		"v1/submit-request.schema.json":         SubmitRequestSchema(),
		"v1/submit-response.schema.json":        SubmitResponseSchema(),
	}
}
