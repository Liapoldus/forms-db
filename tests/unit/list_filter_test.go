package unit

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
	"github.com/Liapoldus/forms-db/internal/presentation/plugin"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
)

func TestListFiltersBySchemaPropertyEquality(t *testing.T) {
	server := newCursorTestServer(t, application.Service{Repository: storage.NewMemoryRepository()})
	settings := []byte(`{"schemas":{"contact":{"type":"object","properties":{"email":{"type":"string"}},"required":["email"],"additionalProperties":false}}}`)
	applyTestConfig(t, server, string(settings))
	submitForm(t, server, `{"site":"portal","schemaName":"contact","data":{"email":"one@example.test"}}`)
	submitForm(t, server, `{"site":"portal","schemaName":"contact","data":{"email":"two@example.test"}}`)

	response, err := server.Call(context.Background(), &pluginv1.CallRequest{
		Capability: "forms.list",
		Payload:    []byte(`{"site":"portal","schemaName":"contact","limit":50,"filter":{"field":"email","equals":"one@example.test"}}`),
		Grants:     []*pluginv1.ActiveGrant{testCursorGrant(t)},
	})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Status int    `json:"status"`
		Body   []byte `json:"body"`
	}
	if err := json.Unmarshal(response.GetPayload(), &envelope); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Items []struct {
			Data map[string]any `json:"data"`
		} `json:"items"`
	}
	if err := json.Unmarshal(envelope.Body, &result); err != nil {
		t.Fatal(err)
	}
	if envelope.Status != 200 || len(result.Items) != 1 || result.Items[0].Data["email"] != "one@example.test" {
		t.Fatalf("filter must return only the matching row: status=%d items=%+v", envelope.Status, result.Items)
	}
}

func TestListRejectsFilterOutsideRegisteredSchema(t *testing.T) {
	server := newCursorTestServer(t, application.Service{Repository: storage.NewMemoryRepository()})
	settings := []byte(`{"schemas":{"contact":{"type":"object","properties":{"email":{"type":"string"}},"additionalProperties":false}}}`)
	applyTestConfig(t, server, string(settings))
	response, err := server.Call(context.Background(), &pluginv1.CallRequest{
		Capability: "forms.list",
		Payload:    []byte(`{"site":"portal","schemaName":"contact","filter":{"field":"password","equals":"secret"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(response.GetPayload(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Status != 422 {
		t.Fatalf("filter outside the registered schema must be rejected: status=%d", envelope.Status)
	}
}

func TestListRejectsUnregisteredSchemaWithoutFilter(t *testing.T) {
	server := newCursorTestServer(t, application.Service{Repository: storage.NewMemoryRepository()})
	response, err := server.Call(context.Background(), &pluginv1.CallRequest{
		Capability: "forms.list",
		Payload:    []byte(`{"site":"portal","schemaName":"missing"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(response.GetPayload(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Status != 422 {
		t.Fatalf("forms.list must reject unregistered schemas: status=%d", envelope.Status)
	}
}

func TestListRejectsLimitOutsideContractBounds(t *testing.T) {
	server := newCursorTestServer(t, application.Service{Repository: storage.NewMemoryRepository()})
	applySettings(t, server, `{"schemas":{"contact":{"type":"object"}}}`)
	for _, limit := range []int{0, 101, -1} {
		payload, err := json.Marshal(map[string]any{"site": "portal", "schemaName": "contact", "limit": limit})
		if err != nil {
			t.Fatal(err)
		}
		response, err := server.Call(context.Background(), &pluginv1.CallRequest{Capability: "forms.list", Payload: payload})
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Status int `json:"status"`
		}
		if err := json.Unmarshal(response.GetPayload(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Status != 422 {
			t.Fatalf("limit %d must be rejected by the v1 contract: status=%d", limit, envelope.Status)
		}
	}
}

func TestListFilterRecognizesPropertyResolvedThroughLocalReference(t *testing.T) {
	server := newCursorTestServer(t, application.Service{Repository: storage.NewMemoryRepository()})
	settings := []byte(`{"schemas":{"contact":{"$schema":"https://json-schema.org/draft/2020-12/schema","$ref":"#/$defs/contact","$defs":{"contact":{"type":"object","properties":{"email":{"type":"string"}},"required":["email"],"additionalProperties":false}}}}}`)
	applyTestConfig(t, server, string(settings))
	submitForm(t, server, `{"site":"portal","schemaName":"contact","data":{"email":"one@example.test"}}`)
	response, err := server.Call(context.Background(), &pluginv1.CallRequest{
		Capability: "forms.list",
		Payload:    []byte(`{"site":"portal","schemaName":"contact","filter":{"field":"email","equals":"one@example.test"}}`),
		Grants:     []*pluginv1.ActiveGrant{testCursorGrant(t)},
	})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(response.GetPayload(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Status != 200 {
		t.Fatalf("filter fields from a local schema reference must be allowed: status=%d", envelope.Status)
	}
	response, err = server.Call(context.Background(), &pluginv1.CallRequest{
		Capability: "forms.list",
		Payload:    []byte(`{"site":"portal","schemaName":"contact","filter":{"field":"password","equals":"not-allowed"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(response.GetPayload(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Status != 422 {
		t.Fatalf("closed local-reference schema must reject undeclared filter fields: status=%d", envelope.Status)
	}
}

func submitForm(t *testing.T, server *plugin.Server, payload string) {
	t.Helper()
	response, err := server.Call(context.Background(), &pluginv1.CallRequest{Capability: "forms.submit", Payload: []byte(payload)})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(response.GetPayload(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Status != 200 {
		t.Fatalf("forms.submit returned status %d", envelope.Status)
	}
}
