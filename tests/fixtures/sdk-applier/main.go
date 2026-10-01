package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
	"github.com/Liapoldus/forms-db/internal/presentation/restplugin"
	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
)

type secretSource struct {
	reference string
	purpose   string
}

func (source *secretSource) SecretProvider(_ context.Context, reference, purpose string) (sdkmodels.SecretValue, error) {
	source.reference, source.purpose = reference, purpose
	return sdkmodels.NewSecretValue([]byte("postgresql://private-dsn")), nil
}

func main() {
	base := storage.NewMemoryRepository()
	secrets := &secretSource{}
	var active interfaces.Repository
	var secretWasScoped bool
	adapter, err := restplugin.New(application.Service{Repository: base}, func(_ context.Context, settings config.Settings) (interfaces.Repository, error) {
		if settings.Driver != "postgres" || string(settings.DSN) != "postgresql://private-dsn" {
			return nil, errors.New("candidate did not receive scoped DSN")
		}
		secretWasScoped = true
		active = storage.NewMemoryRepository()
		return active, nil
	}, secrets)
	if err != nil {
		panic(err)
	}
	good := []byte(`{"driver":"postgres","dsn":"secret:forms-storage","schemas":{}}`)
	configuration, err := sdkmodels.NewConfiguration("generation-1", "1", sdkmodels.Digest(good), good)
	if err != nil || adapter.Apply(context.Background(), configuration) != nil {
		panic("valid candidate rejected")
	}
	accepted := false
	_ = adapter.Use(func(service application.Service, settings config.Settings) error {
		accepted = service.Repository == active && settings.Driver == "postgres"
		return nil
	})
	bad := []byte(`{"driver":"postgres","schemas":{}}`)
	invalid, _ := sdkmodels.NewConfiguration("generation-2", "1", sdkmodels.Digest(bad), bad)
	if adapter.Apply(context.Background(), invalid) == nil {
		panic("invalid candidate accepted")
	}
	preserved := false
	_ = adapter.Use(func(service application.Service, settings config.Settings) error {
		preserved = service.Repository == active && settings.Driver == "postgres"
		return nil
	})
	purpose, _ := config.DSNSecretGrantPurpose()
	result, _ := json.Marshal(map[string]bool{
		"accepted": accepted, "preserved": preserved,
		"secretWasScoped": secretWasScoped && secrets.reference == "secret:forms-storage" && secrets.purpose == purpose,
	})
	fmt.Println(string(result))
}
