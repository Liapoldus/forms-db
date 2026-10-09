package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Liapoldus/forms-db/tests/fixtures/support"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage/memory"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage/sqlstore"
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
	base := memory.New()
	secrets := &secretSource{}
	var active interfaces.Repository
	var secretWasScoped bool
	adapter, err := restplugin.New(application.Service{Repository: base}, func(_ context.Context, settings config.Settings) (interfaces.Repository, error) {
		if settings.Driver != "postgres" || string(settings.DSN) != "postgresql://private-dsn" {
			return nil, errors.New("candidate did not receive scoped DSN")
		}
		if settings.TablePrefix == "forms_fail_" {
			// SQL constructors naturally produce a typed nil pointer on failure.
			// The applier must not call Close on that interface value.
			return (*sqlstore.Repository)(nil), errors.New("candidate repository construction failed")
		}
		secretWasScoped = true
		active = memory.New()
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
	if err := adapter.Use(func(service application.Service, _ config.Settings) error {
		_, err := service.Submit(context.Background(), models.Submission{
			ID: "before-candidate-failure", Site: "site", Schema: "contact", CreatedAt: "2026-10-02T00:00:00Z",
			Data: map[string]any{"email": "retained@example.test"},
		})
		return err
	}); err != nil {
		panic("active repository refused the pre-failure submission")
	}
	accepted := false
	support.Check(adapter.Use(func(service application.Service, settings config.Settings) error {
		accepted = service.Repository == active && settings.Driver == "postgres"
		return nil
	}))
	bad := []byte(`{"driver":"postgres","schemas":{}}`)
	invalid, fixtureErr := sdkmodels.NewConfiguration("generation-2", "1", sdkmodels.Digest(bad), bad)
	support.Check(fixtureErr)
	if adapter.Apply(context.Background(), invalid) == nil {
		panic("invalid candidate accepted")
	}
	preserved := false
	support.Check(adapter.Use(func(service application.Service, settings config.Settings) error {
		preserved = service.Repository == active && settings.Driver == "postgres"
		return nil
	}))
	failingCandidate := []byte(`{"driver":"postgres","dsn":"secret:forms-storage","tablePrefix":"forms_fail_","schemas":{}}`)
	failingConfiguration, fixtureErr := sdkmodels.NewConfiguration("generation-3", "1", sdkmodels.Digest(failingCandidate), failingCandidate)
	support.Check(fixtureErr)
	if adapter.Apply(context.Background(), failingConfiguration) == nil {
		panic("repository construction failure was accepted")
	}
	buildFailurePreserved := false
	servesPriorState := false
	support.Check(adapter.Use(func(service application.Service, settings config.Settings) error {
		buildFailurePreserved = service.Repository == active && settings.Driver == "postgres" && settings.TablePrefix == "form_"
		page, err := service.List(context.Background(), "site", "contact", nil, nil, 10)
		servesPriorState = err == nil && len(page) == 1 && page[0].ID == "before-candidate-failure"
		return nil
	}))
	purpose, _ := config.DSNSecretGrantPurpose()
	result, fixtureErr := json.Marshal(map[string]bool{
		"accepted": accepted, "preserved": preserved, "buildFailurePreserved": buildFailurePreserved,
		"servesPriorState": servesPriorState,
		"secretWasScoped":  secretWasScoped && secrets.reference == "secret:forms-storage" && secrets.purpose == purpose,
	})
	support.Check(fixtureErr)
	support.Written(fmt.Println(string(result)))
}
