package peerplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/contracts"
	"github.com/Liapoldus/forms-db/internal/infrastructure/security"
	"github.com/Liapoldus/forms-db/internal/presentation/restplugin"
	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	"github.com/Liapoldus/pluginprotocol/presentation/peer"
)

// SecretProvider obtains one scoped cursor key per call; the SDK owns the
// Core-facing grant lifecycle, and no secret enters the peer protocol itself.
type SecretProvider interface {
	SecretProvider(context.Context, string, string) (sdkmodels.SecretValue, error)
}

type Handler struct {
	active  *restplugin.Adapter
	secrets SecretProvider
}

func New(active *restplugin.Adapter, secrets SecretProvider, authorizer peer.Authorizer) (peer.Handler, error) {
	if active == nil {
		return nil, restplugin.ErrStorageUnavailable
	}
	handler := &Handler{active: active, secrets: secrets}
	return peer.NewRegistry().WithAuthorizer(authorizer).
		RegisterCall("forms.submit", handler.submit).
		RegisterCall("forms.list", handler.list).
		RegisterCall("forms.delete", handler.delete).
		RegisterCall("admin.surface.get", handler.adminSurface).
		Build()
}

func (handler *Handler) submit(ctx context.Context, call peer.Call) (peer.Result, error) {
	payload, err := requestPayload(call.Payload)
	if err != nil {
		return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
	}
	var input struct {
		Site       string         `json:"site"`
		SchemaName string         `json:"schemaName"`
		Data       map[string]any `json:"data"`
	}
	if decodeObject(payload, &input) != nil || input.Site == "" || input.SchemaName == "" || input.Data == nil {
		return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
	}
	var result peer.Result
	err = handler.active.Use(func(service application.Service, settings config.Settings) error {
		if settings.ValidateSubmissionData(input.SchemaName, input.Data) != nil {
			result = httpJSON(422, map[string]any{"code": "validation_failed"})
			return nil
		}
		item, err := service.Submit(ctx, models.Submission{Site: input.Site, Schema: input.SchemaName, Data: input.Data})
		if err != nil {
			result = httpJSON(503, map[string]any{"code": "storage_unavailable"})
			return nil
		}
		result = httpJSON(200, map[string]any{"id": item.ID, "createdAt": item.CreatedAt, "data": item.Data})
		return nil
	})
	return result, err
}

func (handler *Handler) list(ctx context.Context, call peer.Call) (peer.Result, error) {
	payload, err := requestPayload(call.Payload)
	if err != nil {
		return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
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
		return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
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

func (handler *Handler) delete(ctx context.Context, call peer.Call) (peer.Result, error) {
	payload, err := requestPayload(call.Payload)
	if err != nil {
		return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
	}
	var input struct {
		Site       string `json:"site"`
		SchemaName string `json:"schemaName"`
		ID         string `json:"id"`
	}
	if decodeObject(payload, &input) != nil || input.Site == "" || input.SchemaName == "" || input.ID == "" {
		return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
	}
	var result peer.Result
	err = handler.active.Use(func(service application.Service, _ config.Settings) error {
		if err := service.Delete(ctx, input.Site, input.SchemaName, input.ID); err != nil {
			if errors.Is(err, interfaces.ErrNotFound) {
				result = httpJSON(404, map[string]any{"code": "not_found"})
			} else {
				result = httpJSON(503, map[string]any{"code": "storage_unavailable"})
			}
			return nil
		}
		result = httpJSON(200, map[string]any{"deleted": true, "id": input.ID})
		return nil
	})
	return result, err
}

func (handler *Handler) adminSurface(context.Context, peer.Call) (peer.Result, error) {
	payload, err := contracts.AdminSurface()
	if err != nil {
		return peer.Result{}, peer.ErrInternal
	}
	return peer.Result{Payload: payload}, nil
}

func requestPayload(payload []byte) ([]byte, error) {
	var envelope struct {
		Method     string            `json:"method"`
		Path       string            `json:"path"`
		Query      string            `json:"query"`
		Headers    map[string]string `json:"headers"`
		Body       []byte            `json:"body"`
		RequestID  string            `json:"requestId"`
		RemoteAddr string            `json:"remoteAddr"`
	}
	if decodeObject(payload, &envelope) == nil && envelope.Body != nil {
		return envelope.Body, nil
	}
	return payload, nil
}

func decodeObject(payload []byte, target any) error {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return peer.ErrInvalidRequest
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func httpJSON(status int, value any) peer.Result {
	body, _ := json.Marshal(value)
	payload, _ := json.Marshal(map[string]any{
		"status":  status,
		"headers": map[string]string{"Content-Type": "application/json"},
		"body":    string(body),
	})
	return peer.Result{Payload: payload}
}
