package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Liapoldus/forms-db/tests/fixtures/support"
	"os"
	"strings"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/security"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage/memory"
	"github.com/Liapoldus/forms-db/internal/presentation/peerplugin"
	"github.com/Liapoldus/forms-db/internal/presentation/restplugin"
	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	"github.com/Liapoldus/pluginprotocol/v2/presentation/peer"
)

type secretSource struct {
	scoped      bool
	key         []byte
	unavailable bool
}

func (source *secretSource) SecretProvider(_ context.Context, reference, purpose string) (sdkmodels.SecretValue, error) {
	if source.unavailable {
		return sdkmodels.SecretValue{}, errors.New("scoped secret unavailable")
	}
	_, expectedPurpose, _, ok := security.CursorGrantScope()
	source.scoped = source.scoped || ok && reference == "secret:cursor" && purpose == expectedPurpose
	key := source.key
	if len(key) == 0 {
		key = []byte(strings.Repeat("k", 32))
	}
	return sdkmodels.NewSecretValue(key), nil
}

type response struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

type requestJSONVectors struct {
	InvalidRequests []struct {
		Capability     string `json:"capability"`
		Payload        string `json:"payload"`
		ExpectedStatus int    `json:"expectedStatus"`
		ExpectedCode   string `json:"expectedCode"`
	} `json:"invalidRequests"`
}

type submitNegativeVectors struct {
	Cases []struct {
		Payload        json.RawMessage `json:"payload"`
		ExpectedStatus int             `json:"expectedStatus"`
		ExpectedCode   string          `json:"expectedCode"`
	} `json:"cases"`
}

func main() {
	settings := []byte(`{"driver":"memory","cursorSecretRef":"secret:cursor","schemas":{"contact":{"type":"object","properties":{"email":{"type":"string"}},"required":["email"],"additionalProperties":false}}}`)
	sharedRepository := memory.New()
	firstSecrets := &secretSource{}
	secondSecrets := &secretSource{}
	firstHandler := newHandler(sharedRepository, firstSecrets, settings)
	secondHandler := newHandler(sharedRepository, secondSecrets, settings)
	invoke := func(handler *peerplugin.Handler, method string, body []byte) response {
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
		payload, fixtureErr := json.Marshal(map[string]any{"site": "portal", "schemaName": "contact", "data": map[string]string{"email": email}})
		support.Check(fixtureErr)
		if invoke(firstHandler, "forms.submit", payload).Status != 200 {
			panic("submit failed")
		}
	}
	first := invoke(firstHandler, "forms.list", []byte(`{"site":"portal","schemaName":"contact","limit":1}`))
	var firstBody struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		NextCursor *string `json:"nextCursor"`
	}
	support.Check(json.Unmarshal([]byte(first.Body), &firstBody))
	pageOne := first.Status == 200 && len(firstBody.Items) == 1 && firstBody.NextCursor != nil
	pageTwo := false
	cursorReplayAccepted := false
	tamperRefused := false
	rotatedKeyStatus := 0
	unavailableGrantStatus := 0
	unsupportedVersionStatus := 0
	hostileFilter, fixtureErr := json.Marshal(map[string]any{
		"site": "portal", "schemaName": "contact",
		"filter": map[string]any{"field": `email\" OR 1=1 --`, "equals": "a@example.test"},
	})
	support.Check(fixtureErr)
	var hostileFilterBody struct {
		Code string `json:"code"`
	}
	hostileFilterResult := invoke(firstHandler, "forms.list", hostileFilter)
	hostileFilterRefused := hostileFilterResult.Status == 422 &&
		json.Unmarshal([]byte(hostileFilterResult.Body), &hostileFilterBody) == nil &&
		hostileFilterBody.Code == "validation_failed"
	var vectors requestJSONVectors
	if json.Unmarshal([]byte(os.Getenv("FORMS_REQUEST_JSON_VECTORS")), &vectors) != nil || len(vectors.InvalidRequests) == 0 {
		panic("request JSON conformance vectors unavailable")
	}
	invalidRequestVectorsRefused := true
	for _, vector := range vectors.InvalidRequests {
		switch vector.Capability {
		case "forms.submit", "forms.list", "forms.delete":
		default:
			panic("unexpected capability in request JSON vectors")
		}
		result := invoke(firstHandler, vector.Capability, []byte(vector.Payload))
		var body struct {
			Code string `json:"code"`
		}
		if result.Status != vector.ExpectedStatus || json.Unmarshal([]byte(result.Body), &body) != nil || body.Code != vector.ExpectedCode {
			invalidRequestVectorsRefused = false
		}
	}
	var submitVectors submitNegativeVectors
	if json.Unmarshal([]byte(os.Getenv("FORMS_SUBMIT_NEGATIVE_VECTORS")), &submitVectors) != nil || len(submitVectors.Cases) == 0 {
		panic("submit negative vectors unavailable")
	}
	submitNegativeVectorsRefused := true
	for _, vector := range submitVectors.Cases {
		result := invoke(firstHandler, "forms.submit", vector.Payload)
		var body struct {
			Code string `json:"code"`
		}
		if result.Status != vector.ExpectedStatus || json.Unmarshal([]byte(result.Body), &body) != nil || body.Code != vector.ExpectedCode {
			submitNegativeVectorsRefused = false
		}
	}
	if firstBody.NextCursor != nil {
		secondRequest, fixtureErr := json.Marshal(map[string]any{"site": "portal", "schemaName": "contact", "limit": 1, "cursor": *firstBody.NextCursor})
		support.Check(fixtureErr)
		second := invoke(secondHandler, "forms.list", secondRequest)
		var secondBody struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		support.Check(json.Unmarshal([]byte(second.Body), &secondBody))
		pageTwo = second.Status == 200 && len(secondBody.Items) == 1 && secondBody.Items[0].ID != firstBody.Items[0].ID
		replay := invoke(firstHandler, "forms.list", secondRequest)
		var replayBody struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		support.Check(json.Unmarshal([]byte(replay.Body), &replayBody))
		cursorReplayAccepted = replay.Status == 200 && len(replayBody.Items) == len(secondBody.Items) &&
			len(replayBody.Items) == 1 && replayBody.Items[0].ID == secondBody.Items[0].ID
		tampered, fixtureErr := json.Marshal(map[string]any{"site": "portal", "schemaName": "contact", "limit": 1, "cursor": *firstBody.NextCursor + "x"})
		support.Check(fixtureErr)
		tamperRefused = invoke(secondHandler, "forms.list", tampered).Status == 422
		versionTampered, err := base64.RawURLEncoding.DecodeString(*firstBody.NextCursor)
		if err != nil || len(versionTampered) == 0 {
			panic("could not decode cursor vector")
		}
		versionTampered[0]++
		unsupportedVersionToken := base64.RawURLEncoding.EncodeToString(versionTampered)
		unsupportedVersionRequest, fixtureErr := json.Marshal(map[string]any{
			"site": "portal", "schemaName": "contact", "limit": 1, "cursor": unsupportedVersionToken,
		})
		support.Check(fixtureErr)
		unsupportedVersionStatus = invoke(firstHandler, "forms.list", unsupportedVersionRequest).Status
		rotatedHandler := newHandler(sharedRepository, &secretSource{key: []byte(strings.Repeat("r", 32))}, settings)
		rotatedKeyResult := invoke(rotatedHandler, "forms.list", secondRequest)
		rotatedKeyStatus = rotatedKeyResult.Status
		unavailableHandler := newHandler(sharedRepository, &secretSource{unavailable: true}, settings)
		unavailableResult := invoke(unavailableHandler, "forms.list", secondRequest)
		unavailableGrantStatus = unavailableResult.Status
	}
	encoded, fixtureErr := json.Marshal(map[string]any{
		"pageOne": pageOne, "pageTwo": pageTwo,
		"cursorReplayAccepted": cursorReplayAccepted,
		"tamperRefused":        tamperRefused, "hostileFilterRefused": hostileFilterRefused,
		"secretScoped":                 firstSecrets.scoped && secondSecrets.scoped,
		"invalidRequestVectorsRefused": invalidRequestVectorsRefused,
		"submitNegativeVectorsRefused": submitNegativeVectorsRefused,
		"crossReplicaPage":             pageTwo,
		"rotatedKeyStatus":             rotatedKeyStatus,
		"unavailableGrantStatus":       unavailableGrantStatus,
		"unsupportedVersionStatus":     unsupportedVersionStatus,
	})
	support.Check(fixtureErr)
	support.Written(fmt.Println(string(encoded)))
}

func newHandler(repository interfaces.Repository, secrets *secretSource, settings []byte) *peerplugin.Handler {
	active, err := restplugin.New(application.Service{Repository: memory.New()}, func(context.Context, config.Settings) (interfaces.Repository, error) {
		return repository, nil
	}, nil)
	if err != nil {
		panic(err)
	}
	configuration, err := sdkmodels.NewConfiguration("generation-1", "1", sdkmodels.Digest(settings), settings)
	if err != nil || active.Apply(context.Background(), configuration) != nil {
		panic("active schema unavailable")
	}
	handler, err := peerplugin.New(active, secrets, peer.AllowAll{})
	if err != nil {
		panic(err)
	}
	return handler
}
