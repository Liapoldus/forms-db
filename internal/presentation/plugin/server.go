package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/contracts"
	"github.com/Liapoldus/pluginprotocol"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const name = "forms-db"

type Server struct {
	pluginv1.UnimplementedPluginServiceServer
	mu                sync.RWMutex
	config            config.Settings
	service           application.Service
	activeRepository  interfaces.Repository
	repositoryBuilder RepositoryBuilder
	stop              func()
}

type RepositoryBuilder func(context.Context, config.Settings) (interfaces.Repository, error)

func NewServer(service application.Service, stop func()) *Server {
	return NewServerWithRepositoryBuilder(service, nil, stop)
}

func NewServerWithRepositoryBuilder(service application.Service, builder RepositoryBuilder, stop func()) *Server {
	return &Server{
		config:            config.Settings{Driver: "memory", TablePrefix: "form_"},
		service:           service,
		activeRepository:  service.Repository,
		repositoryBuilder: builder,
		stop:              stop,
	}
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
	return &pluginv1.ConfigSchema{Fields: []*pluginv1.ConfigField{
		{Name: "driver", Type: "string", Options: []string{"memory", "sqlite", "postgres", "mysql"}, DefaultJson: `"memory"`},
		{Name: "dsn", Type: "secret"},
		{Name: "tablePrefix", Type: "string", DefaultJson: `"form_"`},
		{Name: "schemas", Type: "object", DefaultJson: `{}`},
	}}, nil
}

func (s *Server) ConfigApply(ctx context.Context, request *pluginv1.ConfigApplyRequest) (*pluginv1.ConfigApplyResult, error) {
	settings, err := config.Apply(request.GetConfig())
	if err != nil {
		return &pluginv1.ConfigApplyResult{Applied: false}, status.Error(codes.InvalidArgument, "invalid forms-db settings")
	}
	var nextRepository interfaces.Repository
	if s.repositoryBuilder != nil {
		nextRepository, err = s.repositoryBuilder(ctx, settings)
		if err != nil {
			return &pluginv1.ConfigApplyResult{Applied: false}, status.Error(codes.InvalidArgument, "forms-db storage is unavailable")
		}
	} else if settings.Driver != "memory" {
		return &pluginv1.ConfigApplyResult{Applied: false}, status.Error(codes.InvalidArgument, "forms-db storage driver is unavailable")
	}

	s.mu.Lock()
	previousRepository := s.activeRepository
	if nextRepository != nil {
		s.service.Repository = nextRepository
		s.activeRepository = nextRepository
	}
	s.config = settings
	s.mu.Unlock()
	if previousRepository != nextRepository {
		if closer, ok := previousRepository.(io.Closer); ok {
			_ = closer.Close()
		}
	}
	return &pluginv1.ConfigApplyResult{Applied: true}, nil
}

func (s *Server) Shutdown(context.Context, *pluginv1.ShutdownRequest) (*pluginv1.ShutdownResult, error) {
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
			Site       string `json:"site"`
			SchemaName string `json:"schemaName"`
			Limit      *int   `json:"limit"`
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
		limit := 50
		if input.Limit != nil {
			if *input.Limit < 1 || *input.Limit > 100 {
				return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
			}
			limit = *input.Limit
		}
		items, err := service.List(ctx, input.Site, input.SchemaName, filter, limit)
		if err != nil {
			return httpJSON(503, map[string]any{"code": "storage_unavailable"}), nil
		}
		return httpJSON(200, map[string]any{"items": items, "nextCursor": nil}), nil
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
