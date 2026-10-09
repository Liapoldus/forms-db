package peerplugin

import (
	"context"

	productcontracts "github.com/Liapoldus/forms-db/contracts"
	"github.com/Liapoldus/forms-db/internal/application"

	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"

	"github.com/Liapoldus/pluginprotocol/v2/presentation/peer"
)

func (handler *Handler) submit(ctx context.Context, call peer.Call) (peer.Result, error) {
	payload, err := requestPayload(call.Payload)
	if err != nil {
		return httpJSON(422, map[string]any{"code": "validation_failed"}), nil //nolint:nilerr // Product refusals are HTTP envelopes, not peer transport failures.
	}
	limits, err := productcontracts.Limits()
	if err != nil || len(payload) > limits.SubmissionRequestMaxBytes {
		return httpJSON(422, map[string]any{"code": "validation_failed"}), nil //nolint:nilerr // Product refusals are HTTP envelopes, not peer transport failures.
	}
	var input struct {
		Site       string         `json:"site"`
		SchemaName string         `json:"schemaName"`
		Data       map[string]any `json:"data"`
	}
	if decodeObject(payload, &input) != nil || input.Site == "" || input.SchemaName == "" || input.Data == nil {
		return httpJSON(422, map[string]any{"code": "validation_failed"}), nil //nolint:nilerr // Product refusals are HTTP envelopes, not peer transport failures.
	}
	var result peer.Result
	err = handler.active.Use(func(service application.Service, settings config.Settings) error {
		if settings.ValidateSubmissionData(input.SchemaName, input.Data) != nil {
			result = httpJSON(422, map[string]any{"code": "validation_failed"})
			return nil //nolint:nilerr // The validation/storage refusal is already represented by the product HTTP envelope.
		}
		item, err := service.Submit(ctx, models.Submission{Site: input.Site, Schema: input.SchemaName, Data: input.Data})
		if err != nil {
			result = httpJSON(503, map[string]any{"code": "storage_unavailable"})
			return nil //nolint:nilerr // The validation/storage refusal is already represented by the product HTTP envelope.
		}
		result = httpJSON(200, map[string]any{"id": item.ID, "createdAt": item.CreatedAt, "data": item.Data})
		return nil //nolint:nilerr // The validation/storage refusal is already represented by the product HTTP envelope.
	})
	return result, err
}
