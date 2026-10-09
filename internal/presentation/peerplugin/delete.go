package peerplugin

import (
	"context"

	"errors"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"

	"github.com/Liapoldus/forms-db/internal/infrastructure/config"

	"github.com/Liapoldus/pluginprotocol/v2/presentation/peer"
)

func (handler *Handler) delete(ctx context.Context, call peer.Call) (peer.Result, error) {
	payload, err := requestPayload(call.Payload)
	if err != nil {
		return httpJSON(422, map[string]any{"code": "validation_failed"}), nil //nolint:nilerr // Product refusals are HTTP envelopes, not peer transport failures.
	}
	var input struct {
		Site       string `json:"site"`
		SchemaName string `json:"schemaName"`
		ID         string `json:"id"`
	}
	if decodeObject(payload, &input) != nil || input.Site == "" || input.SchemaName == "" || input.ID == "" {
		return httpJSON(422, map[string]any{"code": "validation_failed"}), nil //nolint:nilerr // Product refusals are HTTP envelopes, not peer transport failures.
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
		return nil //nolint:nilerr // The validation/storage refusal is already represented by the product HTTP envelope.
	})
	return result, err
}
