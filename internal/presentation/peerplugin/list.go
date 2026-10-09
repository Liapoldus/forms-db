package peerplugin

import (
	"context"
	"encoding/json"

	"time"

	"github.com/Liapoldus/forms-db/internal/application"

	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"

	"github.com/Liapoldus/forms-db/internal/infrastructure/security"

	"github.com/Liapoldus/pluginprotocol/v2/presentation/peer"
)

func (handler *Handler) list(ctx context.Context, call peer.Call) (peer.Result, error) {
	payload, err := requestPayload(call.Payload)
	if err != nil {
		return httpJSON(422, map[string]any{"code": "validation_failed"}), nil //nolint:nilerr // Product refusals are HTTP envelopes, not peer transport failures.
	}
	var input struct {
		Site       string  `json:"site"`
		SchemaName string  `json:"schemaName"`
		Cursor     *string `json:"cursor"`
		Limit      *int    `json:"limit"`
		Filter     *struct {
			Field  string          `json:"field"`
			Equals json.RawMessage `json:"equals"`
		} `json:"filter"`
	}
	if decodeObject(payload, &input) != nil || input.Site == "" || input.SchemaName == "" {
		return httpJSON(422, map[string]any{"code": "validation_failed"}), nil //nolint:nilerr // Product refusals are HTTP envelopes, not peer transport failures.
	}
	var result peer.Result
	err = handler.active.Use(func(service application.Service, settings config.Settings) error {
		result = handler.listActive(ctx, service, settings, input.Site, input.SchemaName, input.Cursor, input.Limit, input.Filter)
		return nil
	})
	return result, err
}

func (handler *Handler) listActive(ctx context.Context, service application.Service, settings config.Settings, site, schema string, cursor *string, requestedLimit *int, requestedFilter *struct {
	Field  string          `json:"field"`
	Equals json.RawMessage `json:"equals"`
}) peer.Result {
	if !settings.HasSchema(schema) {
		return httpJSON(422, map[string]any{"code": "validation_failed"})
	}
	var filter *models.SubmissionFilter
	if requestedFilter != nil {
		if requestedFilter.Field == "" || len(requestedFilter.Equals) == 0 || !json.Valid(requestedFilter.Equals) || !settings.AllowsFilterField(schema, requestedFilter.Field) {
			return httpJSON(422, map[string]any{"code": "validation_failed"})
		}
		filter = &models.SubmissionFilter{Field: requestedFilter.Field, Equals: requestedFilter.Equals}
	}
	invalidCursorCode, unavailableKeyCode := security.CursorErrorCodes()
	defaultLimit, maxLimit, lookaheadRows, ok := security.CursorPageLimits()
	if !ok {
		return httpJSON(503, map[string]any{"code": unavailableKeyCode})
	}
	limit := defaultLimit
	if requestedLimit != nil {
		if *requestedLimit < 1 || *requestedLimit > maxLimit {
			return httpJSON(422, map[string]any{"code": "validation_failed"})
		}
		limit = *requestedLimit
	}
	signer, err := handler.cursorSigner(ctx, settings.CursorSecretRef)
	if err != nil {
		return httpJSON(503, map[string]any{"code": unavailableKeyCode})
	}
	defer signer.Close()
	scope := security.CursorScope{Site: site, SchemaName: schema, Filter: filter}
	var after *models.SubmissionCursor
	if cursor != nil {
		position, err := signer.Decode(*cursor, scope, time.Now())
		if err != nil {
			return httpJSON(422, map[string]any{"code": invalidCursorCode})
		}
		after = &position
	}
	items, err := service.List(ctx, site, schema, filter, after, limit+lookaheadRows)
	if err != nil {
		return httpJSON(503, map[string]any{"code": "storage_unavailable"})
	}
	var nextCursor *string
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		token, err := signer.Encode(scope, models.SubmissionCursor{CreatedAt: last.CreatedAt, ID: last.ID}, time.Now())
		if err != nil {
			return httpJSON(503, map[string]any{"code": unavailableKeyCode})
		}
		nextCursor = &token
	}
	return httpJSON(200, map[string]any{"items": items, "nextCursor": nextCursor})
}

func (handler *Handler) cursorSigner(ctx context.Context, reference string) (*security.CursorSigner, error) {
	_, purpose, _, ok := security.CursorGrantScope()
	if !ok || reference == "" || handler.secrets == nil {
		return nil, security.ErrCursorKeyUnavailable
	}
	value, err := handler.secrets.SecretProvider(ctx, reference, purpose)
	if err != nil {
		return nil, security.ErrCursorKeyUnavailable
	}
	defer value.Destroy()
	signer, err := security.NewCursorSigner(value.Bytes())
	if err != nil {
		return nil, security.ErrCursorKeyUnavailable
	}
	return signer, nil
}
