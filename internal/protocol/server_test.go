package protocol

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/infrastructure"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
)

func TestServerManifestAndHealthSurface(t *testing.T) {
	server := NewServer(application.Service{Repository: infrastructure.NewMemoryRepository()}, nil)
	manifest, err := server.Manifest(context.Background(), &pluginv1.ManifestRequest{})
	if err != nil || manifest.GetName() != "forms-db" {
		t.Fatalf("unexpected manifest: %#v, %v", manifest, err)
	}
	if len(manifest.GetCapabilities()) != 4 {
		t.Fatalf("unexpected capabilities: %v", manifest.GetCapabilities())
	}
}

func TestServerSubmitUsesHTTPEnvelopeAndMemoryDouble(t *testing.T) {
	server := NewServer(application.Service{Repository: infrastructure.NewMemoryRepository()}, nil)
	body, _ := json.Marshal(map[string]any{"site": "portal", "schemaName": "contact", "data": map[string]any{"name": "fixture"}})
	envelope, _ := json.Marshal(map[string]any{"method": "POST", "path": "/forms", "body": body})
	response, err := server.Call(context.Background(), &pluginv1.CallRequest{Capability: "forms.submit", Payload: envelope})
	if err != nil {
		t.Fatal(err)
	}
	var httpResponse struct {
		Status int    `json:"status"`
		Body   []byte `json:"body"`
	}
	if err := json.Unmarshal(response.GetPayload(), &httpResponse); err != nil {
		t.Fatal(err)
	}
	if httpResponse.Status != 200 {
		t.Fatalf("unexpected HTTP status: %d", httpResponse.Status)
	}
}

func TestServerRejectsUnknownCapability(t *testing.T) {
	server := NewServer(application.Service{Repository: infrastructure.NewMemoryRepository()}, nil)
	response, err := server.Call(context.Background(), &pluginv1.CallRequest{Capability: "forms.unknown", Payload: []byte(`{}`)})
	if err != nil || response.GetCode() != "capability_not_found" {
		t.Fatalf("unexpected rejection: %#v, %v", response, err)
	}
}
