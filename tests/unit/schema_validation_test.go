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

func TestConfigApplyCompilesSchemasAndSubmitValidatesData(t *testing.T) {
	server := plugin.NewServer(application.Service{Repository: storage.NewMemoryRepository()}, nil)
	invalidSchema := []byte(`{"schemas":{"contact":{"type":17}}}`)
	if result, err := server.ConfigApply(context.Background(), &pluginv1.ConfigApplyRequest{Config: invalidSchema}); err == nil || result.GetApplied() {
		t.Fatalf("invalid JSON Schema must not be applied: result=%#v err=%v", result, err)
	}

	settings := []byte(`{"schemas":{"contact":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["email"],"properties":{"email":{"type":"string"}},"additionalProperties":false}}}`)
	result, err := server.ConfigApply(context.Background(), &pluginv1.ConfigApplyRequest{Config: settings})
	if err != nil || !result.GetApplied() {
		t.Fatalf("valid Draft 2020-12 schema was rejected: result=%#v err=%v", result, err)
	}

	assertSubmitStatus(t, server, `{"site":"portal","schemaName":"contact","data":{"name":"missing email"}}`, 422)
	assertSubmitStatus(t, server, `{"site":"portal","schemaName":"contact","data":{"email":"user@example.com","unexpected":true}}`, 422)
	assertSubmitStatus(t, server, `{"site":"portal","schemaName":"contact","data":{"email":"user@example.com"}}`, 200)
}

func assertSubmitStatus(t *testing.T, server *plugin.Server, payload string, expected int) {
	t.Helper()
	response, err := server.Call(context.Background(), &pluginv1.CallRequest{Capability: "forms.submit", Payload: []byte(payload)})
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status int    `json:"status"`
		Body   []byte `json:"body"`
	}
	if err := json.Unmarshal(response.GetPayload(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != expected {
		t.Fatalf("expected submit status %d, got %d", expected, result.Status)
	}
	if expected == 422 {
		var problem struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(result.Body, &problem); err != nil || problem.Code != "validation_failed" {
			t.Fatalf("invalid submission must return validation_failed, body=%s err=%v", result.Body, err)
		}
	}
}
