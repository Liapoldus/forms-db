package unit

import (
	"context"
	"encoding/json"
	"io/fs"
	"testing"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
	"github.com/Liapoldus/forms-db/internal/presentation/plugin"
	"github.com/Liapoldus/pluginprotocol"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
)

func TestAdminSurfaceReturnsProtocolOwnedContract(t *testing.T) {
	expected, err := fs.ReadFile(pluginprotocol.ContractFiles(), "contracts/forms-db/v1/admin-surface.json")
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
