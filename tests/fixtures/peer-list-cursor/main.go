package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/security"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
	"github.com/Liapoldus/forms-db/internal/presentation/peerplugin"
	"github.com/Liapoldus/forms-db/internal/presentation/restplugin"
	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	"github.com/Liapoldus/pluginprotocol/presentation/peer"
)

type secretSource struct{ scoped bool }

func (source *secretSource) SecretProvider(_ context.Context, reference, purpose string) (sdkmodels.SecretValue, error) {
	_, expectedPurpose, _, ok := security.CursorGrantScope()
	source.scoped = source.scoped || ok && reference == "secret:cursor" && purpose == expectedPurpose
	return sdkmodels.NewSecretValue([]byte(strings.Repeat("k", 32))), nil
}

type response struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

func main() {
	active, err := restplugin.New(application.Service{Repository: storage.NewMemoryRepository()}, func(context.Context, config.Settings) (interfaces.Repository, error) {
		return storage.NewMemoryRepository(), nil
	}, nil)
	if err != nil {
		panic(err)
	}
	settings := []byte(`{"driver":"memory","cursorSecretRef":"secret:cursor","schemas":{"contact":{"type":"object","properties":{"email":{"type":"string"}},"required":["email"],"additionalProperties":false}}}`)
	configuration, err := sdkmodels.NewConfiguration("generation-1", "1", sdkmodels.Digest(settings), settings)
	if err != nil || active.Apply(context.Background(), configuration) != nil {
		panic("active schema unavailable")
	}
	secrets := &secretSource{}
	handler, err := peerplugin.New(active, secrets, peer.AllowAll{})
	if err != nil {
		panic(err)
	}
	invoke := func(method string, body []byte) response {
		call := peer.Call{Method: peer.Method(method), Payload: body}
		serve, err := handler.PrepareCall(context.Background(), peer.PeerIdentity{URI: "spiffe://example/server"}, call)
		if err != nil {
			panic(err)
		}
		result, err := serve(context.Background(), call)
		if err != nil {
			panic(err)
		}
		var output response
		if json.Unmarshal(result.Payload, &output) != nil {
			panic("invalid HTTP action")
		}
		return output
	}
	for _, email := range []string{"a@example.test", "b@example.test"} {
		payload, _ := json.Marshal(map[string]any{"site": "portal", "schemaName": "contact", "data": map[string]string{"email": email}})
		if invoke("forms.submit", payload).Status != 200 {
			panic("submit failed")
		}
	}
	first := invoke("forms.list", []byte(`{"site":"portal","schemaName":"contact","limit":1}`))
	var firstBody struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		NextCursor *string `json:"nextCursor"`
	}
	_ = json.Unmarshal([]byte(first.Body), &firstBody)
	pageOne := first.Status == 200 && len(firstBody.Items) == 1 && firstBody.NextCursor != nil
	pageTwo := false
	tamperRefused := false
	if firstBody.NextCursor != nil {
		secondRequest, _ := json.Marshal(map[string]any{"site": "portal", "schemaName": "contact", "limit": 1, "cursor": *firstBody.NextCursor})
		second := invoke("forms.list", secondRequest)
		var secondBody struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		_ = json.Unmarshal([]byte(second.Body), &secondBody)
		pageTwo = second.Status == 200 && len(secondBody.Items) == 1 && secondBody.Items[0].ID != firstBody.Items[0].ID
		tampered, _ := json.Marshal(map[string]any{"site": "portal", "schemaName": "contact", "limit": 1, "cursor": *firstBody.NextCursor + "x"})
		tamperRefused = invoke("forms.list", tampered).Status == 422
	}
	encoded, _ := json.Marshal(map[string]bool{
		"pageOne": pageOne, "pageTwo": pageTwo,
		"tamperRefused": tamperRefused, "secretScoped": secrets.scoped,
	})
	fmt.Println(string(encoded))
}
