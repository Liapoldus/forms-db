package unit

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Liapoldus/forms-db/internal/application"
	contractadapter "github.com/Liapoldus/forms-db/internal/infrastructure/contracts"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
	"github.com/Liapoldus/forms-db/internal/presentation/plugin"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
)

func TestAdminSurfaceReturnsPluginOwnedContract(t *testing.T) {
	expected, err := contractadapter.AdminSurface()
	if err != nil {
		t.Fatal(err)
	}
	server := plugin.NewServer(application.Service{Repository: storage.NewMemoryRepository()}, nil)
	response, err := server.Call(context.Background(), &pluginv1.CallRequest{Capability: "admin.surface.get"})
	if err != nil {
		t.Fatal(err)
	}
	var expectedValue, actualValue any
	if err := json.Unmarshal(expected, &expectedValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(response.GetPayload(), &actualValue); err != nil {
		t.Fatal(err)
	}
	expectedJSON, _ := json.Marshal(expectedValue)
	actualJSON, _ := json.Marshal(actualValue)
	if string(actualJSON) != string(expectedJSON) {
		t.Fatalf("admin surface must match the protocol-owned fixture: got=%s", actualJSON)
	}
}

func TestAdminSurfaceActionsReferenceDeclaredCallCapabilities(t *testing.T) {
	server := plugin.NewServer(application.Service{Repository: storage.NewMemoryRepository()}, nil)
	manifest, err := server.Manifest(context.Background(), &pluginv1.ManifestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	declared := make(map[string]pluginv1.InvocationMode, len(manifest.GetCapabilityDescriptors()))
	for _, descriptor := range manifest.GetCapabilityDescriptors() {
		if descriptor == nil || len(descriptor.GetModes()) != 1 {
			t.Fatal("manifest capability must have exactly one declared invocation mode")
		}
		declared[descriptor.GetCapability()] = descriptor.GetModes()[0]
	}

	contract, err := contractadapter.AdminSurface()
	if err != nil {
		t.Fatal(err)
	}
	var surface struct {
		Pages []struct {
			Sections []struct {
				Actions []struct {
					Capability string `json:"capability"`
				} `json:"actions"`
			} `json:"sections"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(contract, &surface); err != nil {
		t.Fatal(err)
	}
	for _, page := range surface.Pages {
		for _, section := range page.Sections {
			for _, action := range section.Actions {
				mode, exists := declared[action.Capability]
				if !exists {
					t.Errorf("admin action capability %q is absent from Manifest", action.Capability)
					continue
				}
				if mode != pluginv1.InvocationMode_INVOCATION_MODE_CALL {
					t.Errorf("admin action capability %q must be declared for Call, got %v", action.Capability, mode)
				}
			}
		}
	}
}
