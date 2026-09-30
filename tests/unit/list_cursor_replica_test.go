package unit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
	"github.com/Liapoldus/forms-db/internal/presentation/plugin"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
)

type cursorReplicaVector struct {
	Name           string `json:"name"`
	ExpectedStatus int    `json:"expectedStatus"`
}

type cursorReplicaVectors struct {
	Version   int `json:"version"`
	Vectors   []cursorReplicaVector
	Semantics struct {
		KeyDistribution         string `json:"keyDistribution"`
		Rotation                string `json:"rotation"`
		ReplicaFailure          string `json:"replicaFailure"`
		SettingsRevisionBinding string `json:"settingsRevisionBinding"`
		TokenVersion            string `json:"tokenVersion"`
	} `json:"semantics"`
}

func TestFormsListCursorReplicaVectors(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "contracts", "v1", "cursor-replica-vectors.json"))
	if err != nil {
		t.Fatal("read cursor replica vectors")
	}
	var vectors cursorReplicaVectors
	if err := json.Unmarshal(data, &vectors); err != nil || vectors.Version != 1 {
		t.Fatalf("decode cursor replica vectors: %v", err)
	}
	statuses := make(map[string]int, len(vectors.Vectors))
	for _, vector := range vectors.Vectors {
		statuses[vector.Name] = vector.ExpectedStatus
	}
	for _, required := range []string{
		"shared-current-key-cross-replica",
		"old-cursor-after-key-rotation",
		"cursor-grant-unavailable-on-one-replica",
		"unsupported-token-version",
	} {
		if _, exists := statuses[required]; !exists {
			t.Fatalf("cursor replica vector %q is missing", required)
		}
	}
	for _, semantic := range []string{
		vectors.Semantics.KeyDistribution,
		vectors.Semantics.Rotation,
		vectors.Semantics.ReplicaFailure,
		vectors.Semantics.SettingsRevisionBinding,
		vectors.Semantics.TokenVersion,
	} {
		if semantic == "" {
			t.Fatal("cursor replica semantics must document key sharing, rotation, failure, revision binding, and token version")
		}
	}

	sharedKey := []byte(strings.Repeat("shared-key-12345", 2))
	if len(sharedKey) != 32 {
		t.Fatalf("test key has unexpected length %d", len(sharedKey))
	}
	primary := newCursorReplicaServer(t, seedCursorReplicaRepository(t), sharedKey, true)
	secondary := newCursorReplicaServer(t, seedCursorReplicaRepository(t), sharedKey, true)
	rotated := newCursorReplicaServer(t, seedCursorReplicaRepository(t), []byte(strings.Repeat("rotate-key-12345", 2)), true)
	unavailable := newCursorReplicaServer(t, seedCursorReplicaRepository(t), nil, false)

	firstPage := callListPage(t, primary, `{"site":"portal","schemaName":"contact","limit":1}`)
	if firstPage.NextCursor == nil {
		t.Fatal("first replica must issue a cursor for the next page")
	}
	sharedStatus, sharedPage := callCursorPage(t, secondary, *firstPage.NextCursor)
	if sharedStatus != statuses["shared-current-key-cross-replica"] || len(sharedPage.Items) != 1 || sharedPage.Items[0].ID != "frm_b" {
		t.Fatalf("replica with the same current logical key must continue the page: status=%d page=%+v", sharedStatus, sharedPage)
	}

	rotatedStatus, _ := callCursorPage(t, rotated, *firstPage.NextCursor)
	if rotatedStatus != statuses["old-cursor-after-key-rotation"] {
		t.Fatalf("rotated key must reject old cursors without fallback: status=%d", rotatedStatus)
	}

	unavailableStatus, _ := callCursorPage(t, unavailable, *firstPage.NextCursor)
	if unavailableStatus != statuses["cursor-grant-unavailable-on-one-replica"] {
		t.Fatalf("replica without grant must fail closed: status=%d", unavailableStatus)
	}
	if page := callListPage(t, primary, `{"site":"portal","schemaName":"contact","limit":1}`); len(page.Items) != 1 {
		t.Fatalf("one unavailable replica must not disable a healthy replica: page=%+v", page)
	}

	unsupported := withUnsupportedCursorVersion(t, *firstPage.NextCursor)
	unsupportedStatus, _ := callCursorPage(t, secondary, unsupported)
	if unsupportedStatus != statuses["unsupported-token-version"] {
		t.Fatalf("unsupported token version must fail closed as a validation error: status=%d", unsupportedStatus)
	}
}

func seedCursorReplicaRepository(t *testing.T) *storage.MemoryRepository {
	t.Helper()
	repository := storage.NewMemoryRepository()
	for _, submission := range []models.Submission{
		{ID: "frm_a", CreatedAt: "2026-02-03T04:05:04Z", Site: "portal", Schema: "contact", Data: map[string]any{"email": "a@example.test"}},
		{ID: "frm_b", CreatedAt: "2026-02-03T04:05:05Z", Site: "portal", Schema: "contact", Data: map[string]any{"email": "b@example.test"}},
		{ID: "frm_c", CreatedAt: "2026-02-03T04:05:06Z", Site: "portal", Schema: "contact", Data: map[string]any{"email": "c@example.test"}},
	} {
		if _, err := repository.Submit(context.Background(), submission); err != nil {
			t.Fatal("seed replica repository")
		}
	}
	return repository
}

func newCursorReplicaServer(t *testing.T, repository *storage.MemoryRepository, key []byte, grantAvailable bool) *plugin.Server {
	t.Helper()
	var redeemer plugin.GrantRedeemer
	if grantAvailable {
		redeemer = staticGrantRedeemer{key: append([]byte(nil), key...)}
	}
	server := plugin.NewServerWithRepositoryBuilderAndGrantRedeemer(application.Service{Repository: repository}, nil, redeemer, nil)
	bootstrapTestServer(t, server, redeemer)
	applyTestConfig(t, server, `{"schemas":{"contact":{"type":"object","properties":{"email":{"type":"string"}}}}}`)
	return server
}

func callCursorPage(t *testing.T, server *plugin.Server, cursor string) (int, listPage) {
	t.Helper()
	request, err := json.Marshal(map[string]any{"site": "portal", "schemaName": "contact", "limit": 1, "cursor": cursor})
	if err != nil {
		t.Fatal("encode cursor page request")
	}
	response, err := server.Call(context.Background(), &pluginv1.CallRequest{
		Capability: "forms.list", Payload: request, Grants: []*pluginv1.ActiveGrant{testCursorGrant(t)},
	})
	if err != nil {
		t.Fatal("call forms.list")
	}
	var envelope struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(response.GetPayload(), &envelope); err != nil {
		t.Fatal("decode forms.list envelope")
	}
	var page listPage
	if envelope.Body != "" {
		if err := json.Unmarshal([]byte(envelope.Body), &page); err != nil {
			t.Fatal("decode forms.list page")
		}
	}
	return envelope.Status, page
}

func withUnsupportedCursorVersion(t *testing.T, cursor string) string {
	t.Helper()
	encoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || len(encoded) == 0 {
		t.Fatal("decode test cursor")
	}
	encoded[0]++
	return base64.RawURLEncoding.EncodeToString(encoded)
}
