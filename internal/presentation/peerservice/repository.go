// Package peerservice dispatches authorized product calls.
package peerservice

import (
	"context"

	"github.com/Liapoldus/forms-db/internal/presentation/restservice"
	sdkmodels "github.com/Liapoldus/service-sdk/v2/domain/models"
	sdkpresentation "github.com/Liapoldus/service-sdk/v2/presentation"
	"github.com/Liapoldus/wool/v3/presentation/peer"
)

// SecretProvider obtains one scoped cursor key per call; the SDK owns the
// Core-facing grant lifecycle, and no secret enters the peer protocol itself.
type SecretProvider interface {
	SecretProvider(context.Context, string, string) (sdkmodels.SecretValue, error)
}

type Handler struct {
	peer.Handler
	active  *restservice.Adapter
	secrets SecretProvider
}

func New(active *restservice.Adapter, secrets SecretProvider, authorizer peer.Authorizer) (*Handler, error) {
	if active == nil {
		return nil, restservice.ErrStorageUnavailable
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
