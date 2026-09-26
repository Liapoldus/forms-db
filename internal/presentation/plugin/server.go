package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/contracts"
	"github.com/Liapoldus/forms-db/internal/infrastructure/security"
	"github.com/Liapoldus/pluginprotocol"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
	"github.com/Liapoldus/pluginprotocol/transport"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const name = "forms-db"

type Server struct {
	pluginv1.UnimplementedPluginServiceServer
	mu                sync.RWMutex
	configApplyMu     sync.Mutex
	config            config.Settings
	service           application.Service
	activeRepository  interfaces.Repository
	repositoryBuilder RepositoryBuilder
	grantRedeemer     GrantRedeemer
	grantDialer       GrantBrokerDialer
	instanceID        string
	stop              func()
}

type RepositoryBuilder func(context.Context, config.Settings) (interfaces.Repository, error)

type GrantRedeemer interface {
	Redeem(ctx context.Context, capability, handle, purpose, domain string) ([]byte, error)
	RedeemConfig(ctx context.Context, grant *pluginv1.ActiveGrant) ([]byte, error)
}

type GrantBrokerDialer func(context.Context, *pluginv1.BootstrapRequest) (GrantRedeemer, error)

func NewServer(service application.Service, stop func()) *Server {
	return NewServerWithRepositoryBuilderAndGrantRedeemer(service, nil, nil, stop)
}

func NewServerWithRepositoryBuilder(service application.Service, builder RepositoryBuilder, stop func()) *Server {
	return NewServerWithRepositoryBuilderAndGrantRedeemer(service, builder, nil, stop)
}

func NewServerWithRepositoryBuilderAndGrantRedeemer(service application.Service, builder RepositoryBuilder, redeemer GrantRedeemer, stop func()) *Server {
	return &Server{
		config:            config.Settings{Driver: "memory", TablePrefix: "form_"},
		service:           service,
		activeRepository:  service.Repository,
		repositoryBuilder: builder,
		grantRedeemer:     redeemer,
		grantDialer: func(ctx context.Context, request *pluginv1.BootstrapRequest) (GrantRedeemer, error) {
			return transport.DialGrantBrokerFromBootstrapContext(ctx, request, nil)
		},
		stop: stop,
	}
}

func (s *Server) SetGrantBrokerDialer(dialer GrantBrokerDialer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.grantDialer = dialer
}

func (s *Server) Bootstrap(ctx context.Context, request *pluginv1.BootstrapRequest) (*pluginv1.BootstrapResult, error) {
	s.configApplyMu.Lock()
	defer s.configApplyMu.Unlock()
	if request == nil || request.GetInstanceId() == "" {
		return &pluginv1.BootstrapResult{Accepted: false}, status.Error(codes.InvalidArgument, "invalid plugin bootstrap")
	}
	var redeemer GrantRedeemer
	if request.GetGrantBrokerEndpoint() != "" {
		s.mu.RLock()
		dialer := s.grantDialer
		s.mu.RUnlock()
		if dialer == nil {
			return &pluginv1.BootstrapResult{Accepted: false}, status.Error(codes.Unavailable, "grant broker is unavailable")
		}
		var err error
		redeemer, err = dialer(ctx, request)
		if err != nil {
			return &pluginv1.BootstrapResult{Accepted: false}, status.Error(codes.Unavailable, "grant broker is unavailable")
		}
	}
	s.mu.Lock()
	previous := s.grantRedeemer
	s.grantRedeemer = redeemer
	s.instanceID = request.GetInstanceId()
	s.mu.Unlock()
	if closer, ok := previous.(io.Closer); ok {
		_ = closer.Close()
	}
	return &pluginv1.BootstrapResult{Accepted: true}, nil
}

func (s *Server) Manifest(context.Context, *pluginv1.ManifestRequest) (*pluginv1.Manifest, error) {
	capabilities := []string{"forms.submit", "forms.list", "forms.delete", "admin.surface.get"}
	descriptors := make([]*pluginv1.CapabilityDescriptor, 0, len(capabilities))
	for _, capability := range capabilities {
		descriptors = append(descriptors, &pluginv1.CapabilityDescriptor{
			Capability: capability,
			Modes:      []pluginv1.InvocationMode{pluginv1.InvocationMode_INVOCATION_MODE_CALL},
		})
	}
	return &pluginv1.Manifest{
		Name:                  name,
		ProtocolVersion:       pluginprotocol.ProtocolVersion,
		Capabilities:          capabilities,
		CapabilityDescriptors: descriptors,
	}, nil
}

func (s *Server) ConfigSchema(context.Context, *pluginv1.ConfigSchemaRequest) (*pluginv1.ConfigSchema, error) {
	dsnDescription, ok := config.DSNSecretDescription()
	if !ok {
		return nil, status.Error(codes.Internal, "forms-db secret settings contract is unavailable")
	}
	return &pluginv1.ConfigSchema{Fields: []*pluginv1.ConfigField{
		{Name: "driver", Type: "string", Options: []string{"memory", "sqlite", "postgres", "mysql"}, DefaultJson: `"memory"`},
		{Name: "dsn", Type: "secret", Description: dsnDescription},
		{Name: "tablePrefix", Type: "string", DefaultJson: `"form_"`},
		{Name: "schemas", Type: "object", DefaultJson: `{}`},
	}}, nil
}

func (s *Server) ConfigApply(ctx context.Context, request *pluginv1.ConfigApplyRequest) (*pluginv1.ConfigApplyResult, error) {
	s.configApplyMu.Lock()
	defer s.configApplyMu.Unlock()
	if err := contextError(ctx); err != nil {
		return &pluginv1.ConfigApplyResult{Applied: false}, status.Error(codes.Canceled, "configuration apply canceled")
	}
	if request == nil || request.GetSettingsRevision() == "" {
		return &pluginv1.ConfigApplyResult{Applied: false}, status.Error(codes.InvalidArgument, "invalid forms-db settings revision")
	}
	settings, err := config.Apply(request.GetConfig())
	if err != nil {
		return &pluginv1.ConfigApplyResult{Applied: false, SettingsRevision: request.GetSettingsRevision()}, status.Error(codes.InvalidArgument, "invalid forms-db settings")
	}
	defer func() { clearSecret(settings.DSN) }()
	s.mu.RLock()
	instanceID, redeemer := s.instanceID, s.grantRedeemer
	s.mu.RUnlock()
	if instanceID == "" {
		return &pluginv1.ConfigApplyResult{Applied: false, SettingsRevision: request.GetSettingsRevision()}, status.Error(codes.FailedPrecondition, "plugin bootstrap is required")
	}
	if settings.DSNReference != "" {
		if settings.Driver == "memory" {
			return &pluginv1.ConfigApplyResult{Applied: false, SettingsRevision: request.GetSettingsRevision()}, status.Error(codes.InvalidArgument, "invalid forms-db settings")
		}
		purpose, purposeAvailable := config.DSNSecretGrantPurpose()
		grant, ok := matchingConfigGrant(request.GetGrants(), instanceID, request.GetSettingsRevision(), settings.DSNReference, purpose)
		if !purposeAvailable || !ok || redeemer == nil {
			return &pluginv1.ConfigApplyResult{Applied: false, SettingsRevision: request.GetSettingsRevision()}, status.Error(codes.PermissionDenied, "storage secret grant is unavailable")
		}
		secret, err := redeemer.RedeemConfig(ctx, grant)
		if err != nil {
			return &pluginv1.ConfigApplyResult{Applied: false, SettingsRevision: request.GetSettingsRevision()}, status.Error(codes.PermissionDenied, "storage secret grant is unavailable")
		}
		defer clearSecret(secret)
		if err := settings.ApplyDSNSecret(secret); err != nil {
			return &pluginv1.ConfigApplyResult{Applied: false, SettingsRevision: request.GetSettingsRevision()}, status.Error(codes.InvalidArgument, "invalid storage secret grant")
		}
	}
	if settings.Driver != "memory" && len(settings.DSN) == 0 {
		return &pluginv1.ConfigApplyResult{Applied: false, SettingsRevision: request.GetSettingsRevision()}, status.Error(codes.InvalidArgument, "storage secret grant is unavailable")
	}
	var nextRepository interfaces.Repository
	if s.repositoryBuilder != nil {
		nextRepository, err = s.repositoryBuilder(ctx, settings)
		if err != nil || nextRepository == nil {
			return &pluginv1.ConfigApplyResult{Applied: false, SettingsRevision: request.GetSettingsRevision()}, status.Error(codes.InvalidArgument, "forms-db storage is unavailable")
		}
	} else if settings.Driver != "memory" {
		return &pluginv1.ConfigApplyResult{Applied: false, SettingsRevision: request.GetSettingsRevision()}, status.Error(codes.InvalidArgument, "forms-db storage driver is unavailable")
	}

	s.mu.Lock()
	previousRepository := s.activeRepository
	if nextRepository != nil {
		s.service.Repository = nextRepository
		s.activeRepository = nextRepository
	}
	clearSecret(settings.DSN)
	settings.DSN = nil
	s.config = settings
	s.mu.Unlock()
	if previousRepository != nextRepository {
		if closer, ok := previousRepository.(io.Closer); ok {
			_ = closer.Close()
		}
	}
	return &pluginv1.ConfigApplyResult{Applied: true, SettingsRevision: request.GetSettingsRevision()}, nil
}

func matchingConfigGrant(grants []*pluginv1.ActiveGrant, instanceID, revision, reference, purpose string) (*pluginv1.ActiveGrant, bool) {
	var match *pluginv1.ActiveGrant
	for _, grant := range grants {
		if grant == nil || grant.GetScope() != pluginv1.GrantScope_GRANT_SCOPE_CONFIG_APPLY ||
			grant.GetInstanceId() != instanceID || grant.GetSettingsRevision() != revision || grant.GetSecretReference() != reference ||
			grant.GetPurpose() != purpose || grant.GetHandle() == "" || grant.GetCapability() != "" || len(grant.GetDomains()) != 0 {
			continue
		}
		if match != nil {
			return nil, false
		}
		match = grant
	}
	return match, match != nil
}

func (s *Server) Shutdown(context.Context, *pluginv1.ShutdownRequest) (*pluginv1.ShutdownResult, error) {
	s.configApplyMu.Lock()
	defer s.configApplyMu.Unlock()
	s.mu.Lock()
	if closer, ok := s.grantRedeemer.(io.Closer); ok {
		_ = closer.Close()
	}
	s.grantRedeemer = nil
	s.mu.Unlock()
	if s.stop != nil {
		go s.stop()
	}
	return &pluginv1.ShutdownResult{Closed: true}, nil
}

func (s *Server) Call(ctx context.Context, request *pluginv1.CallRequest) (*pluginv1.CallResponse, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	service, settings := s.service, s.config
	switch request.GetCapability() {
	case "forms.submit":
		payload, err := requestPayload(request.GetPayload())
		if err != nil {
			return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
		}
		var input struct {
			Site       string         `json:"site"`
			SchemaName string         `json:"schemaName"`
			Data       map[string]any `json:"data"`
		}
		if err := decodeObject(payload, &input); err != nil || input.Site == "" || input.SchemaName == "" || input.Data == nil {
			return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
		}
		if err := settings.ValidateSubmissionData(input.SchemaName, input.Data); err != nil {
			return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
		}
		item, err := service.Submit(ctx, models.Submission{Site: input.Site, Schema: input.SchemaName, Data: input.Data})
		if err != nil {
			return httpJSON(503, map[string]any{"code": "storage_unavailable"}), nil
		}
		return httpJSON(200, map[string]any{"id": item.ID, "createdAt": item.CreatedAt, "data": item.Data}), nil
	case "forms.list":
		payload, err := requestPayload(request.GetPayload())
		if err != nil {
			return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
		}
		var input struct {
			Site       string  `json:"site"`
			SchemaName string  `json:"schemaName"`
			Cursor     *string `json:"cursor"`
			Limit      *int    `json:"limit"`
			Filter     *struct {
				Field  string          `json:"field"`
				Equals json.RawMessage `json:"equals"`
			} `json:"filter"`
		}
		if err := decodeObject(payload, &input); err != nil || input.Site == "" || input.SchemaName == "" || !settings.HasSchema(input.SchemaName) {
			return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
		}
		var filter *models.SubmissionFilter
		if input.Filter != nil {
			if input.Filter.Field == "" || len(input.Filter.Equals) == 0 || !json.Valid(input.Filter.Equals) || !settings.AllowsFilterField(input.SchemaName, input.Filter.Field) {
				return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
			}
			filter = &models.SubmissionFilter{Field: input.Filter.Field, Equals: input.Filter.Equals}
		}
		invalidCursorCode, unavailableKeyCode := security.CursorErrorCodes()
		defaultLimit, maxLimit, lookaheadRows, limitsAvailable := security.CursorPageLimits()
		if !limitsAvailable {
			return httpJSON(503, map[string]any{"code": unavailableKeyCode}), nil
		}
		limit := defaultLimit
		if input.Limit != nil {
			if *input.Limit < 1 || *input.Limit > maxLimit {
				return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
			}
			limit = *input.Limit
		}
		var after *models.SubmissionCursor
		scope := security.CursorScope{Site: input.Site, SchemaName: input.SchemaName, Filter: filter}
		signer, err := s.cursorSignerForCall(ctx, request.GetCapability(), request.GetGrants())
		if err != nil {
			return httpJSON(503, map[string]any{"code": unavailableKeyCode}), nil
		}
		defer signer.Close()
		if input.Cursor != nil {
			position, err := signer.Decode(*input.Cursor, scope, time.Now())
			if err != nil {
				return httpJSON(422, map[string]any{"code": invalidCursorCode}), nil
			}
			after = &position
		}
		items, err := service.List(ctx, input.Site, input.SchemaName, filter, after, limit+lookaheadRows)
		if err != nil {
			return httpJSON(503, map[string]any{"code": "storage_unavailable"}), nil
		}
		var nextCursor *string
		if len(items) > limit {
			items = items[:limit]
			last := items[len(items)-1]
			token, err := signer.Encode(scope, models.SubmissionCursor{CreatedAt: last.CreatedAt, ID: last.ID}, time.Now())
			if err != nil {
				return httpJSON(503, map[string]any{"code": unavailableKeyCode}), nil
			}
			nextCursor = &token
		}
		return httpJSON(200, map[string]any{"items": items, "nextCursor": nextCursor}), nil
	case "forms.delete":
		payload, err := requestPayload(request.GetPayload())
		if err != nil {
			return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
		}
		var input struct {
			Site       string `json:"site"`
			SchemaName string `json:"schemaName"`
			ID         string `json:"id"`
		}
		if err := decodeObject(payload, &input); err != nil || input.ID == "" {
			return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
		}
		if input.Site == "" || input.SchemaName == "" {
			return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
		}
		if err := service.Delete(ctx, input.Site, input.SchemaName, input.ID); err != nil {
			if !errors.Is(err, interfaces.ErrNotFound) {
				return httpJSON(503, map[string]any{"code": "storage_unavailable"}), nil
			}
			return httpJSON(404, map[string]any{"code": "not_found"}), nil
		}
		return httpJSON(200, map[string]any{"deleted": true, "id": input.ID}), nil
	case "admin.surface.get":
		surface, err := contracts.AdminSurface()
		if err != nil {
			return nil, status.Error(codes.Internal, "admin surface contract is unavailable")
		}
		return &pluginv1.CallResponse{Payload: surface}, nil
	default:
		return rejected("capability_not_found", "capability is not declared by forms-db"), nil
	}
}

func (s *Server) cursorSignerForCall(ctx context.Context, capability string, grants []*pluginv1.ActiveGrant) (*security.CursorSigner, error) {
	grantCapability, purpose, domain, ok := security.CursorGrantScope()
	if !ok || capability != grantCapability || s.grantRedeemer == nil {
		return nil, security.ErrCursorKeyUnavailable
	}
	var matched *pluginv1.ActiveGrant
	for _, grant := range grants {
		if grant == nil || grant.GetScope() != pluginv1.GrantScope_GRANT_SCOPE_CALL || grant.GetCapability() != grantCapability || grant.GetPurpose() != purpose || grant.GetHandle() == "" {
			continue
		}
		for _, allowedDomain := range grant.GetDomains() {
			if allowedDomain == domain {
				if matched != nil {
					return nil, security.ErrCursorKeyUnavailable
				}
				matched = grant
				break
			}
		}
	}
	if matched == nil {
		return nil, security.ErrCursorKeyUnavailable
	}
	key, err := s.grantRedeemer.Redeem(ctx, grantCapability, matched.GetHandle(), purpose, domain)
	if err != nil {
		return nil, security.ErrCursorKeyUnavailable
	}
	defer clearSecret(key)
	signer, err := security.NewCursorSigner(key)
	if err != nil {
		return nil, security.ErrCursorKeyUnavailable
	}
	return signer, nil
}

func clearSecret(secret []byte) {
	for index := range secret {
		secret[index] = 0
	}
}

func contextError(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func decodeObject(payload []byte, target any) error {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("payload must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func requestPayload(payload []byte) ([]byte, error) {
	var envelope struct {
		Method     string            `json:"method"`
		Path       string            `json:"path"`
		Query      string            `json:"query"`
		Headers    map[string]string `json:"headers"`
		Body       []byte            `json:"body"`
		RequestID  string            `json:"requestId"`
		RemoteAddr string            `json:"remoteAddr"`
	}
	if err := decodeObject(payload, &envelope); err == nil && envelope.Body != nil {
		return envelope.Body, nil
	}
	return payload, nil
}

func rejected(code, message string) *pluginv1.CallResponse {
	return &pluginv1.CallResponse{Code: code, Message: message}
}

func jsonResponse(value any) *pluginv1.CallResponse {
	payload, _ := json.Marshal(value)
	return &pluginv1.CallResponse{Payload: payload}
}

func httpJSON(status int, value any) *pluginv1.CallResponse {
	body, _ := json.Marshal(value)
	return jsonResponse(map[string]any{"status": status, "headers": map[string]string{"Content-Type": "application/json"}, "body": body})
}
