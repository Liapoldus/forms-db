// Package restplugin adapts the Plugin SDK lifecycle to forms-db.
package restplugin

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/Liapoldus/forms-db/contracts"
	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
)

var ErrInvalidSettings = errors.New("invalid forms-db settings")
var ErrStorageUnavailable = errors.New("forms-db storage unavailable")

// RepositoryBuilder prepares a complete candidate before it can become active.
// On error it closes any partially created repository itself and returns no
// usable repository to the caller.
type RepositoryBuilder func(context.Context, config.Settings) (interfaces.Repository, error)

// SecretProvider redeems one generation-scoped secret through the Plugin SDK.
// The caller owns and destroys the returned value after the candidate is built.
type SecretProvider interface {
	SecretProvider(context.Context, string, string) (sdkmodels.SecretValue, error)
}

// Adapter owns the product state. The Plugin SDK sees only its opaque metadata
// and ConfigurationApplier interface, never its driver, schema or DSN.
type Adapter struct {
	mu         sync.RWMutex
	service    application.Service
	settings   config.Settings
	repository interfaces.Repository
	build      RepositoryBuilder
	secrets    SecretProvider
}

// Use holds the active repository stable across one product invocation, so a
// concurrent Reload cannot close it while the invocation is still using it.
func (adapter *Adapter) Use(run func(application.Service, config.Settings) error) error {
	if adapter == nil || run == nil {
		return ErrStorageUnavailable
	}
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()
	return run(adapter.service, adapter.settings)
}

func New(service application.Service, build RepositoryBuilder, secrets SecretProvider) (*Adapter, error) {
	if service.Repository == nil || build == nil {
		return nil, ErrStorageUnavailable
	}
	return &Adapter{service: service, repository: service.Repository, build: build, secrets: secrets}, nil
}

func (adapter *Adapter) SetSecretProvider(secrets SecretProvider) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	adapter.secrets = secrets
}

// SecretProvider forwards the active SDK grant broker to product handlers. The
// adapter may be constructed before the SDK lifecycle initializes this broker;
// callers fail closed until the lifecycle has installed it.
func (adapter *Adapter) SecretProvider(ctx context.Context, reference, purpose string) (sdkmodels.SecretValue, error) {
	if adapter == nil {
		return sdkmodels.SecretValue{}, ErrStorageUnavailable
	}
	adapter.mu.RLock()
	secrets := adapter.secrets
	adapter.mu.RUnlock()
	if secrets == nil {
		return sdkmodels.SecretValue{}, ErrStorageUnavailable
	}
	return secrets.SecretProvider(ctx, reference, purpose)
}

func (adapter *Adapter) Manifest(context.Context) ([]byte, error) {
	return contracts.PluginManifest()
}

func (adapter *Adapter) ConfigurationSchema(context.Context) ([]byte, error) {
	return contracts.SettingsSchema()
}

func (adapter *Adapter) Apply(ctx context.Context, incoming sdkmodels.Configuration) error {
	if adapter == nil || ctx.Err() != nil || incoming.Validate() != nil {
		return ErrInvalidSettings
	}
	raw := incoming.Bytes()
	if contracts.ValidateSettings(raw) != nil {
		return ErrInvalidSettings
	}
	settings, err := config.Apply(raw)
	if err != nil {
		return ErrInvalidSettings
	}
	defer clearSecret(settings.DSN)
	if settings.Driver != "memory" {
		adapter.mu.RLock()
		secrets := adapter.secrets
		adapter.mu.RUnlock()
		purpose, ok := config.DSNSecretGrantPurpose()
		if !ok || secrets == nil || settings.DSNReference == "" {
			return ErrStorageUnavailable
		}
		value, err := secrets.SecretProvider(ctx, settings.DSNReference, purpose)
		if err != nil {
			return ErrStorageUnavailable
		}
		defer value.Destroy()
		if settings.ApplyDSNSecret(value.Bytes()) != nil {
			return ErrStorageUnavailable
		}
	}
	if ctx.Err() != nil {
		return ErrStorageUnavailable
	}
	candidate, err := adapter.build(ctx, settings)
	if err != nil || candidate == nil {
		// A failed builder may return a typed nil inside the repository interface.
		// It owns cleanup on failure; calling Close here could panic before Reload
		// clears its applying generation.
		return ErrStorageUnavailable
	}
	if ctx.Err() != nil {
		if closer, ok := candidate.(io.Closer); ok {
			_ = closer.Close() //nolint:errcheck // A canceled candidate is discarded; cleanup cannot make it publishable.
		}
		return ErrStorageUnavailable
	}
	clearSecret(settings.DSN)
	settings.DSN = nil
	adapter.mu.Lock()
	previous := adapter.repository
	adapter.repository = candidate
	adapter.service.Repository = candidate
	adapter.settings = settings
	adapter.mu.Unlock()
	if previous != candidate {
		if closer, ok := previous.(io.Closer); ok {
			_ = closer.Close() //nolint:errcheck // Active state has switched; a retired repository close failure cannot roll it back.
		}
	}
	return nil
}

func clearSecret(secret []byte) {
	for index := range secret {
		secret[index] = 0
	}
}
