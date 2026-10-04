package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"time"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
	"github.com/Liapoldus/forms-db/internal/presentation/restplugin"
	sdkinterfaces "github.com/Liapoldus/plugin-sdk/domain/interfaces"
	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	sdkinfra "github.com/Liapoldus/plugin-sdk/infrastructure"
	sdkpresentation "github.com/Liapoldus/plugin-sdk/presentation"
)

type source struct {
	document sdkmodels.Configuration
	exact    bool
}

func (value *source) PullExact(_ context.Context, generation string) (sdkinterfaces.PullResult, error) {
	value.exact = generation == value.document.Generation
	return sdkinterfaces.PullResult{Configuration: value.document, State: sdkinterfaces.GenerationStateActive}, nil
}

type broker struct {
	candidate bool
}

func (value *broker) IssueGrant(_ context.Context, request sdkmodels.SecretGrantRequest) (sdkmodels.SecretGrant, error) {
	value.candidate = request.Generation == "generation-1" && request.Reference == "secret:forms-storage"
	return sdkmodels.SecretGrant{
		Handle: "grant-1", Reference: request.Reference, Purpose: request.Purpose,
		Generation: request.Generation, ExpiresAt: time.Now().Add(time.Minute),
	}, nil
}

func (*broker) Redeem(context.Context, sdkmodels.SecretRedemption) (sdkmodels.SecretValue, error) {
	return sdkmodels.NewSecretValue([]byte("postgresql://private-dsn")), nil
}

func main() {
	raw := []byte(`{"driver":"postgres","dsn":"secret:forms-storage","schemas":{}}`)
	document, err := sdkmodels.NewConfiguration("generation-1", "1", sdkmodels.Digest(raw), raw)
	if err != nil {
		panic(err)
	}
	configurationSource := &source{document: document}
	secretBroker := &broker{}
	active, err := restplugin.New(application.Service{Repository: storage.NewMemoryRepository()}, func(_ context.Context, settings config.Settings) (interfaces.Repository, error) {
		if settings.Driver != "postgres" || string(settings.DSN) != "postgresql://private-dsn" {
			panic("candidate does not have scoped DSN")
		}
		return storage.NewMemoryRepository(), nil
	}, nil)
	if err != nil {
		panic(err)
	}
	identity, err := sdkmodels.NewReplicaIdentity("forms-db", "replica-1")
	if err != nil {
		panic(err)
	}
	handler, lifecycle, _, err := restplugin.NewHandler(active, restplugin.LifecycleOptions{
		Source: configurationSource, Broker: secretBroker, Identity: identity, LogOutput: io.Discard,
		AdminSurface: testAdminSurface{}, AdminActions: testAdminActions{},
	})
	if err != nil {
		panic(err)
	}
	contract, err := sdkinfra.LoadHTTPContract()
	if err != nil {
		panic(err)
	}
	endpoint, err := contract.Endpoint("reload")
	if err != nil {
		panic("reload endpoint missing")
	}
	reload, _ := json.Marshal(sdkmodels.Reload{Generation: document.Generation, SHA256: document.SHA256, SchemaVersion: document.SchemaVersion})
	request := httptest.NewRequest(endpoint.Method, endpoint.Path, bytes.NewReader(reload))
	request.Header.Set("content-type", contract.Plugin.ReloadRequest.MediaType)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var acknowledgement sdkmodels.ReloadAcknowledgement
	_ = json.Unmarshal(response.Body.Bytes(), &acknowledgement)
	output, _ := json.Marshal(map[string]bool{
		"applied":        acknowledgement.Applied && acknowledgement.Generation == document.Generation,
		"exactPull":      configurationSource.exact,
		"candidateGrant": secretBroker.candidate,
		"ready":          lifecycle.Readiness().Ready && lifecycle.Readiness().Generation == document.Generation,
	})
	fmt.Println(string(output))
}

type testAdminSurface struct{}

func (testAdminSurface) AdminSurface(context.Context) ([]byte, error) {
	return []byte(`{"version":1}`), nil
}

type testAdminActions struct{}

func (testAdminActions) HandleAdminAction(context.Context, sdkpresentation.AdminActionInput) (sdkpresentation.AdminActionResponse, error) {
	return sdkpresentation.AdminActionResponse{StatusCode: 200, Body: []byte(`{"ok":true}`)}, nil
}
