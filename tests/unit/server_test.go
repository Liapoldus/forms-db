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

func TestServerManifestAndHealthSurface(t *testing.T) {
	server := plugin.NewServer(application.Service{Repository: storage.NewMemoryRepository()}, nil)
	manifest, err := server.Manifest(context.Background(), &pluginv1.ManifestRequest{})
	if err != nil || manifest.GetName() != "forms-db" {
		t.Fatalf("unexpected manifest: %#v, %v", manifest, err)
	}
	if len(manifest.GetCapabilities()) != 4 {
		t.Fatalf("unexpected capabilities: %v", manifest.GetCapabilities())
	}
}

func TestServerManifestDeclaresCallModeForEveryCapability(t *testing.T) {
	server := plugin.NewServer(application.Service{Repository: storage.NewMemoryRepository()}, nil)
	manifest, err := server.Manifest(context.Background(), &pluginv1.ManifestRequest{})
	if err != nil {
		t.Fatal(err)
	}

	descriptors := manifest.GetCapabilityDescriptors()
	if len(descriptors) != len(manifest.GetCapabilities()) {
		t.Fatalf("manifest has %d capabilities but %d descriptors", len(manifest.GetCapabilities()), len(descriptors))
	}

	modesByCapability := make(map[string][]pluginv1.InvocationMode, len(descriptors))
	for _, descriptor := range descriptors {
		if descriptor == nil {
			t.Fatal("manifest contains a nil capability descriptor")
		}
		capability := descriptor.GetCapability()
		if _, exists := modesByCapability[capability]; exists {
			t.Fatalf("manifest contains duplicate descriptor for %q", capability)
		}
		modesByCapability[capability] = descriptor.GetModes()
	}

	for _, capability := range manifest.GetCapabilities() {
		modes, exists := modesByCapability[capability]
		if !exists {
			t.Errorf("capability %q has no invocation descriptor", capability)
			continue
		}
		if len(modes) != 1 || modes[0] != pluginv1.InvocationMode_INVOCATION_MODE_CALL {
			t.Errorf("capability %q has unexpected invocation modes: %v", capability, modes)
		}
		delete(modesByCapability, capability)
	}
	for capability := range modesByCapability {
		t.Errorf("manifest describes undeclared capability %q", capability)
	}
}

func TestServerSubmitUsesHTTPEnvelopeAndMemoryDouble(t *testing.T) {
	server := plugin.NewServer(application.Service{Repository: storage.NewMemoryRepository()}, nil)
	bootstrapTestServer(t, server, nil)
	settings := []byte(`{"schemas":{"contact":{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}}}`)
	applyTestConfig(t, server, string(settings))
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
	server := plugin.NewServer(application.Service{Repository: storage.NewMemoryRepository()}, nil)
	response, err := server.Call(context.Background(), &pluginv1.CallRequest{Capability: "forms.unknown", Payload: []byte(`{}`)})
	if err != nil || response.GetCode() != "capability_not_found" {
		t.Fatalf("unexpected rejection: %#v, %v", response, err)
	}
}
