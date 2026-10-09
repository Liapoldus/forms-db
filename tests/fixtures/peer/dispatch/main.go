package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Liapoldus/forms-db/tests/fixtures/support"
	"os"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage/memory"
	"github.com/Liapoldus/forms-db/internal/presentation/peerplugin"
	"github.com/Liapoldus/forms-db/internal/presentation/restplugin"
	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	"github.com/Liapoldus/pluginprotocol/v2/presentation/peer"
)

type deleteNegativeVector struct {
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
}

func main() {
	active, err := restplugin.New(application.Service{Repository: memory.New()}, func(context.Context, config.Settings) (interfaces.Repository, error) {
		return memory.New(), nil
	}, nil)
	if err != nil {
		panic(err)
	}
	settings := []byte(`{"driver":"memory","schemas":{"contact":{"type":"object","properties":{"email":{"type":"string"}},"required":["email"],"additionalProperties":false}}}`)
	configuration, err := sdkmodels.NewConfiguration("generation-1", "1", sdkmodels.Digest(settings), settings)
	if err != nil || active.Apply(context.Background(), configuration) != nil {
		panic("active form schema unavailable")
	}
	handler, err := peerplugin.New(active, nil, peer.AllowAll{})
	if err != nil {
		panic(err)
	}
	invoke := func(method string, payload []byte) (int, string) {
		call := peer.Call{Method: peer.Method(method), Payload: payload}
		serve, err := handler.PrepareCall(context.Background(), peer.PeerIdentity{URI: "spiffe://example/server"}, call)
		if err != nil {
			panic(err)
		}
		result, err := serve(context.Background(), call)
		if err != nil {
			panic(err)
		}
		var response struct {
			Status int    `json:"status"`
			Body   string `json:"body"`
		}
		if json.Unmarshal(result.Payload, &response) != nil {
			panic("invalid HTTP action")
		}
		return response.Status, response.Body
	}
	status, body := invoke("forms.submit", []byte(`{"site":"portal","schemaName":"contact","data":{"email":"a@example.test"}}`))
	envelopeBody := []byte(`{"site":"portal","schemaName":"contact","data":{"email":"b@example.test"}}`)
	envelope, fixtureErr := json.Marshal(map[string]any{
		"method": "POST", "path": "/forms", "headers": map[string]string{"content-type": "application/json"},
		"cookies": []map[string]string{{"name": "session", "value": "allowed-context-marker"}},
		"body":    envelopeBody, "requestId": "request-2", "remoteAddr": "127.0.0.1:1",
	})
	support.Check(fixtureErr)
	envelopeStatus, _ := invoke("forms.submit", envelope)
	var submitted struct {
		ID string `json:"id"`
	}
	support.Check(json.Unmarshal([]byte(body), &submitted))
	deletedStatus, _ := invoke("forms.delete", []byte(fmt.Sprintf(`{"site":"portal","schemaName":"contact","id":%q}`, submitted.ID)))
	absentStatus, _ := invoke("forms.delete", []byte(fmt.Sprintf(`{"site":"portal","schemaName":"contact","id":%q}`, submitted.ID)))
	var deleteVectors []deleteNegativeVector
	if json.Unmarshal([]byte(os.Getenv("FORMS_DELETE_NEGATIVE_VECTORS")), &deleteVectors) != nil || len(deleteVectors) == 0 {
		panic("delete negative vectors unavailable")
	}
	deleteNegativeVectorsRefused := true
	for _, vector := range deleteVectors {
		if vector.Name == "" || len(vector.Payload) == 0 {
			deleteNegativeVectorsRefused = false
			continue
		}
		resultStatus, resultBody := invoke("forms.delete", vector.Payload)
		var errorBody struct {
			Code string `json:"code"`
		}
		if resultStatus != 422 || json.Unmarshal([]byte(resultBody), &errorBody) != nil || errorBody.Code != "validation_failed" {
			deleteNegativeVectorsRefused = false
		}
	}
	encoded, fixtureErr := json.Marshal(map[string]bool{
		"submitted":                    status == 200 && submitted.ID != "",
		"httpEnvelope":                 envelopeStatus == 200,
		"deleted":                      deletedStatus == 200,
		"absent":                       absentStatus == 404,
		"deleteNegativeVectorsRefused": deleteNegativeVectorsRefused,
	})
	support.Check(fixtureErr)
	support.Written(fmt.Println(string(encoded)))
}
