package unit

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/domain/models"
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
		Body   string `json:"body"`
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

func TestFormsDeleteDistinguishesMissingRecordsFromStorageFailures(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		deleteErr  error
		wantStatus int
		wantCode   string
	}{
		{name: "missing record", deleteErr: interfaces.ErrNotFound, wantStatus: 404, wantCode: "not_found"},
		{name: "storage failure", deleteErr: errors.New("database unavailable"), wantStatus: 503, wantCode: "storage_unavailable"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repository := deleteFailureRepository{Repository: storage.NewMemoryRepository(), deleteErr: testCase.deleteErr}
			server := plugin.NewServer(application.Service{Repository: repository}, nil)
			requestBody, err := json.Marshal(map[string]string{"site": "portal", "schemaName": "contact", "id": "frm_123"})
			if err != nil {
				t.Fatal(err)
			}
			payload, err := json.Marshal(struct {
				Method string `json:"method"`
				Path   string `json:"path"`
				Body   []byte `json:"body"`
			}{Method: "DELETE", Path: "/forms/submissions/frm_123", Body: requestBody})
			if err != nil {
				t.Fatal(err)
			}
			response, err := server.Call(context.Background(), &pluginv1.CallRequest{Capability: "forms.delete", Payload: payload})
			if err != nil {
				t.Fatal(err)
			}
			var actual struct {
				Status int    `json:"status"`
				Body   string `json:"body"`
			}
			if err := json.Unmarshal(response.GetPayload(), &actual); err != nil {
				t.Fatal(err)
			}
			var responseBody struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal([]byte(actual.Body), &responseBody); err != nil {
				t.Fatal(err)
			}
			if actual.Status != testCase.wantStatus || responseBody.Code != testCase.wantCode {
				t.Fatalf("delete response = (%d, %q), want (%d, %q)", actual.Status, responseBody.Code, testCase.wantStatus, testCase.wantCode)
			}
		})
	}
}

func TestFormsDeleteRepeatedDeletionReturnsNotFound(t *testing.T) {
	repository := storage.NewMemoryRepository()
	if _, err := repository.Submit(context.Background(), models.Submission{
		ID: "frm_123", Site: "portal", Schema: "contact", Data: map[string]any{"name": "Ada"},
	}); err != nil {
		t.Fatal(err)
	}
	server := plugin.NewServer(application.Service{Repository: repository}, nil)

	if statusCode, code := callDelete(t, server); statusCode != 200 || code != "" {
		t.Fatalf("first delete response = (%d, %q), want (200, empty code)", statusCode, code)
	}
	if statusCode, code := callDelete(t, server); statusCode != 404 || code != "not_found" {
		t.Fatalf("repeated delete response = (%d, %q), want (404, not_found)", statusCode, code)
	}
}

func TestHTTPResponseActionBodyIsUTF8JSONStringInCallResponse(t *testing.T) {
	server := plugin.NewServer(application.Service{Repository: storage.NewMemoryRepository()}, nil)
	requestBody, err := json.Marshal(map[string]string{"site": "portal", "schemaName": "contact", "id": "missing"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		Body   []byte `json:"body"`
	}{Method: "DELETE", Path: "/forms/submissions/missing", Body: requestBody})
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Call(context.Background(), &pluginv1.CallRequest{Capability: "forms.delete", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	var action struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(response.GetPayload(), &action); err != nil {
		t.Fatal(err)
	}
	if action.Status != 404 {
		t.Fatalf("unexpected status: %d", action.Status)
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(action.Body), &body); err != nil {
		t.Fatalf("response action body is not a UTF-8 JSON string: %v; body=%q", err, action.Body)
	}
	if body.Code != "not_found" {
		t.Fatalf("unexpected response action body: %q", action.Body)
	}
}

func callDelete(t *testing.T, server *plugin.Server) (int, string) {
	t.Helper()
	body, err := json.Marshal(map[string]string{"site": "portal", "schemaName": "contact", "id": "frm_123"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		Body   []byte `json:"body"`
	}{Method: "DELETE", Path: "/forms/submissions/frm_123", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Call(context.Background(), &pluginv1.CallRequest{Capability: "forms.delete", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	var actual struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(response.GetPayload(), &actual); err != nil {
		t.Fatal(err)
	}
	var responseBody struct {
		Code string `json:"code"`
	}
	if actual.Body != "" {
		if err := json.Unmarshal([]byte(actual.Body), &responseBody); err != nil {
			t.Fatal(err)
		}
	}
	return actual.Status, responseBody.Code
}

type deleteFailureRepository struct {
	interfaces.Repository
	deleteErr error
}

func (r deleteFailureRepository) Delete(context.Context, string, string, string) error {
	return r.deleteErr
}

var _ interfaces.Repository = deleteFailureRepository{}
