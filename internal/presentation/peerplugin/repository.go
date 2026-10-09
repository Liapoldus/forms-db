// Package peerplugin dispatches authorized product calls.
package peerplugin

import (
	"context"

	"github.com/Liapoldus/forms-db/internal/presentation/restplugin"
	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	sdkpresentation "github.com/Liapoldus/plugin-sdk/presentation"
	"github.com/Liapoldus/pluginprotocol/v2/presentation/peer"
)

// SecretProvider obtains one scoped cursor key per call; the SDK owns the
// Core-facing grant lifecycle, and no secret enters the peer protocol itself.
type SecretProvider interface {
	SecretProvider(context.Context, string, string) (sdkmodels.SecretValue, error)
}

type Handler struct {
	peer.Handler
	active  *restplugin.Adapter
	secrets SecretProvider
}

func New(active *restplugin.Adapter, secrets SecretProvider, authorizer peer.Authorizer) (*Handler, error) {
	if active == nil {
		return nil, restplugin.ErrStorageUnavailable
	}
	handler := &Handler{active: active, secrets: secrets}
	registered, err := peer.NewRegistry().WithAuthorizer(authorizer).
		RegisterCall("forms.submit", handler.submit).
		RegisterCall("forms.list", handler.list).
		RegisterCall("forms.delete", handler.delete).
		Build()
	if err != nil {
		return nil, err
	}
	handler.Handler = registered
	return handler, nil
}

var _ peer.Handler = (*Handler)(nil)

var _ sdkpresentation.AdminSurfaceProvider = (*Handler)(nil)

var _ sdkpresentation.AdminActionHandler = (*Handler)(nil)
