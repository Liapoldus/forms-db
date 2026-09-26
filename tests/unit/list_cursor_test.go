package unit

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/security"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
	"github.com/Liapoldus/forms-db/internal/presentation/plugin"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
)

func TestFormsListUsesStableScopedKeysetPages(t *testing.T) {
	repository := storage.NewMemoryRepository()
	createdAt := "2026-02-03T04:05:06Z"
	for _, item := range []models.Submission{
		{ID: "frm_a", CreatedAt: createdAt, Site: "portal", Schema: "contact", Data: map[string]any{"email": "match@example.test"}},
		{ID: "frm_b", CreatedAt: createdAt, Site: "portal", Schema: "contact", Data: map[string]any{"email": "match@example.test"}},
		{ID: "frm_c", CreatedAt: createdAt, Site: "portal", Schema: "contact", Data: map[string]any{"email": "match@example.test"}},
		{ID: "frm_other", CreatedAt: createdAt, Site: "other", Schema: "contact", Data: map[string]any{"email": "match@example.test"}},
		{ID: "frm_filter", CreatedAt: createdAt, Site: "portal", Schema: "contact", Data: map[string]any{"email": "other@example.test"}},
	} {
		if _, err := repository.Submit(context.Background(), item); err != nil {
			t.Fatal("seed submission failed")
		}
	}
	server := newCursorTestServer(t, application.Service{Repository: repository})
	applySettings(t, server, `{"schemas":{"contact":{"type":"object","properties":{"email":{"type":"string"}}}}}`)
	payload := `{"site":"portal","schemaName":"contact","limit":2,"filter":{"field":"email","equals":"match@example.test"}}`
	first := callListPage(t, server, payload)
	if len(first.Items) != 2 || first.NextCursor == nil || first.Items[0].ID != "frm_c" || first.Items[1].ID != "frm_b" {
		t.Fatalf("first page should be ordered by createdAt/id descending: %+v", first)
	}
	var next map[string]any
	if err := json.Unmarshal([]byte(payload), &next); err != nil {
		t.Fatal("decode test request")
	}
	next["cursor"] = *first.NextCursor
	applySettings(t, server, `{"schemas":{"contact":{"type":"object","properties":{"email":{"type":"string"}}}}}`)
	encoded, err := json.Marshal(next)
	if err != nil {
		t.Fatal("encode next-page request")
	}
	second := callListPage(t, server, string(encoded))
	if len(second.Items) != 1 || second.NextCursor != nil || second.Items[0].ID != "frm_a" {
		t.Fatalf("second page must continue strictly after prior position: %+v", second)
	}
}

func TestFormsListRejectsTamperedAndWrongScopeCursorsWithoutEchoing(t *testing.T) {
	repository := storage.NewMemoryRepository()
	for _, id := range []string{"frm_a", "frm_b"} {
		if _, err := repository.Submit(context.Background(), models.Submission{
			ID: id, CreatedAt: "2026-02-03T04:05:06Z", Site: "portal", Schema: "contact",
			Data: map[string]any{"email": "a@example.test"},
		}); err != nil {
			t.Fatal("seed submission failed")
		}
	}
	server := newCursorTestServer(t, application.Service{Repository: repository})
	applySettings(t, server, `{"schemas":{"contact":{"type":"object","properties":{"email":{"type":"string"}}}}}`)
	page := callListPage(t, server, `{"site":"portal","schemaName":"contact","limit":1}`)
	if page.NextCursor == nil {
		t.Fatal("fixture must produce another page")
	}
	for _, request := range []map[string]any{
		{"site": "other", "schemaName": "contact", "limit": 1, "cursor": *page.NextCursor},
		{"site": "portal", "schemaName": "contact", "limit": 1, "filter": map[string]any{"field": "email", "equals": "a@example.test"}, "cursor": *page.NextCursor},
		{"site": "portal", "schemaName": "contact", "limit": 1, "cursor": tamperCursor(*page.NextCursor)},
	} {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatal("encode invalid cursor request")
		}
		response, err := server.Call(context.Background(), &pluginv1.CallRequest{Capability: "forms.list", Payload: encoded, Grants: []*pluginv1.ActiveGrant{testCursorGrant(t)}})
		if err != nil {
			t.Fatal("forms.list transport call failed")
		}
		var envelope struct {
			Status int    `json:"status"`
			Body   string `json:"body"`
		}
		if err := json.Unmarshal(response.GetPayload(), &envelope); err != nil {
			t.Fatal("decode forms.list response")
		}
		if envelope.Status != 422 {
			t.Fatalf("invalid cursor must return generic validation error: status=%d body=%s", envelope.Status, envelope.Body)
		}
		if envelope.Body != "" && strings.Contains(envelope.Body, fmt.Sprint(request["cursor"])) {
			t.Fatal("response must not echo cursor contents")
		}
	}
}

func TestFormsListWithoutCursorKeyFailsClosed(t *testing.T) {
	server := plugin.NewServer(application.Service{Repository: storage.NewMemoryRepository()}, nil)
	bootstrapTestServer(t, server, nil)
	applySettings(t, server, `{"schemas":{"contact":{"type":"object"}}}`)
	response, err := server.Call(context.Background(), &pluginv1.CallRequest{
		Capability: "forms.list", Payload: []byte(`{"site":"portal","schemaName":"contact"}`), Grants: []*pluginv1.ActiveGrant{testCursorGrant(t)},
	})
	if err != nil {
		t.Fatal("forms.list transport call failed")
	}
	var envelope struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(response.GetPayload(), &envelope); err != nil {
		t.Fatal("decode forms.list response")
	}
	if envelope.Status != 503 {
		t.Fatalf("missing signing key must fail closed: status=%d", envelope.Status)
	}
}

type listPage struct {
	Items      []models.Submission `json:"items"`
	NextCursor *string             `json:"nextCursor"`
}

func callListPage(t *testing.T, server *plugin.Server, payload string) listPage {
	t.Helper()
	response, err := server.Call(context.Background(), &pluginv1.CallRequest{Capability: "forms.list", Payload: []byte(payload), Grants: []*pluginv1.ActiveGrant{testCursorGrant(t)}})
	if err != nil {
		t.Fatal("forms.list transport call failed")
	}
	var envelope struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(response.GetPayload(), &envelope); err != nil || envelope.Status != 200 {
		t.Fatalf("forms.list failed: status=%d err=%v", envelope.Status, err)
	}
	var result listPage
	if err := json.Unmarshal([]byte(envelope.Body), &result); err != nil {
		t.Fatal("decode list page")
	}
	return result
}

func newCursorTestServer(t *testing.T, service application.Service) *plugin.Server {
	t.Helper()
	return newCursorTestServerWithBuilder(t, service, nil)
}

func newCursorTestServerWithBuilder(t *testing.T, service application.Service, builder plugin.RepositoryBuilder) *plugin.Server {
	return newCursorTestServerWithBuilderAndConfigSecrets(t, service, builder, nil)
}

func newCursorTestServerWithBuilderAndConfigSecrets(t *testing.T, service application.Service, builder plugin.RepositoryBuilder, configSecrets map[string][]byte) *plugin.Server {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("generate ephemeral test key")
	}
	defer func() {
		for index := range key {
			key[index] = 0
		}
	}()
	redeemer := staticGrantRedeemer{key: append([]byte(nil), key...), configSecrets: configSecrets}
	server := plugin.NewServerWithRepositoryBuilderAndGrantRedeemer(service, builder, redeemer, nil)
	bootstrapTestServer(t, server, redeemer)
	return server
}

type staticGrantRedeemer struct {
	key           []byte
	configSecrets map[string][]byte
}

func (r staticGrantRedeemer) Redeem(_ context.Context, capability, handle, purpose, domain string) ([]byte, error) {
	grantCapability, grantPurpose, grantDomain, ok := security.CursorGrantScope()
	if !ok || capability != grantCapability || handle == "" || purpose != grantPurpose || domain != grantDomain {
		return nil, security.ErrCursorKeyUnavailable
	}
	return append([]byte(nil), r.key...), nil
}

func (r staticGrantRedeemer) RedeemConfig(_ context.Context, grant *pluginv1.ActiveGrant) ([]byte, error) {
	purpose, ok := config.DSNSecretGrantPurpose()
	if grant == nil || !ok || grant.GetScope() != pluginv1.GrantScope_GRANT_SCOPE_CONFIG_APPLY || grant.GetHandle() == "" ||
		grant.GetPurpose() != purpose || grant.GetInstanceId() != testPluginInstanceID || grant.GetSettingsRevision() == "" ||
		grant.GetCapability() != "" || len(grant.GetDomains()) != 0 {
		return nil, security.ErrCursorKeyUnavailable
	}
	secret, ok := r.configSecrets[grant.GetSecretReference()]
	if !ok {
		return nil, security.ErrCursorKeyUnavailable
	}
	return append([]byte(nil), secret...), nil
}

func tamperCursor(token string) string {
	tamperAt := len(token) / 2
	if token[tamperAt] == 'A' {
		return token[:tamperAt] + "B" + token[tamperAt+1:]
	}
	return token[:tamperAt] + "A" + token[tamperAt+1:]
}
