package peerplugin

import (
	"context"
	"encoding/json"

	"github.com/Liapoldus/forms-db/internal/infrastructure/contracts"

	sdkpresentation "github.com/Liapoldus/plugin-sdk/presentation"
	"github.com/Liapoldus/pluginprotocol/v2/presentation/peer"
)

func (handler *Handler) AdminSurface(context.Context) ([]byte, error) {
	return contracts.AdminSurface()
}

// HandleAdminAction adapts the product-owned query and delete operations to the
// generic Plugin SDK management action transport. The SDK owns REST/mTLS and
// invocation validation; forms-db owns the page/action mapping and payloads.
func (handler *Handler) HandleAdminAction(ctx context.Context, input sdkpresentation.AdminActionInput) (sdkpresentation.AdminActionResponse, error) {
	if input.Invocation.PageID != "submissions" {
		return sdkpresentation.AdminActionResponse{StatusCode: 404, Body: []byte(`{"code":"not_found"}`)}, nil
	}
	var result peer.Result
	var err error
	switch input.Invocation.ActionID {
	case "query":
		result, err = handler.list(ctx, peer.Call{Payload: input.Body})
	case "delete":
		result, err = handler.delete(ctx, peer.Call{Payload: input.Body})
	default:
		return sdkpresentation.AdminActionResponse{StatusCode: 404, Body: []byte(`{"code":"not_found"}`)}, nil
	}
	if err != nil {
		return sdkpresentation.AdminActionResponse{}, peer.ErrInternal
	}
	var response struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if json.Unmarshal(result.Payload, &response) != nil || response.Status == 0 || !json.Valid([]byte(response.Body)) {
		return sdkpresentation.AdminActionResponse{}, peer.ErrInternal
	}
	return sdkpresentation.AdminActionResponse{StatusCode: response.Status, Body: []byte(response.Body)}, nil
}
