package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const name = "forms-db"

type Server struct {
	pluginv1.UnimplementedPluginServiceServer
	mu      sync.RWMutex
	config  config.Settings
	service application.Service
	stop    func()
}

func NewServer(service application.Service, stop func()) *Server {
	return &Server{config: config.Settings{Driver: "memory", TablePrefix: "form_"}, service: service, stop: stop}
}

func (s *Server) Manifest(context.Context, *pluginv1.ManifestRequest) (*pluginv1.Manifest, error) {
	return &pluginv1.Manifest{Name: name, ProtocolVersion: "liapoldus.plugin.v1", Capabilities: []string{"forms.submit", "forms.list", "forms.delete", "admin.surface.get"}}, nil
}

func (s *Server) ConfigSchema(context.Context, *pluginv1.ConfigSchemaRequest) (*pluginv1.ConfigSchema, error) {
	return &pluginv1.ConfigSchema{Fields: []*pluginv1.ConfigField{
		{Name: "driver", Type: "string", Options: []string{"memory", "sqlite", "postgres", "mysql"}, DefaultJson: `"memory"`},
		{Name: "dsn", Type: "secret"},
		{Name: "tablePrefix", Type: "string", DefaultJson: `"form_"`},
		{Name: "schemas", Type: "object", DefaultJson: `{}`},
	}}, nil
}

func (s *Server) ConfigApply(_ context.Context, request *pluginv1.ConfigApplyRequest) (*pluginv1.ConfigApplyResult, error) {
	settings, err := config.Apply(request.GetConfig())
	if err != nil {
		return &pluginv1.ConfigApplyResult{Applied: false}, status.Error(codes.InvalidArgument, "invalid forms-db settings")
	}
	s.mu.Lock()
	s.config = settings
	s.mu.Unlock()
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
		item, err := s.service.Submit(ctx, models.Submission{Site: input.Site, Schema: input.SchemaName, Data: input.Data})
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
			Limit      int    `json:"limit"`
		}
		if err := decodeObject(payload, &input); err != nil || input.Site == "" || input.SchemaName == "" {
			return httpJSON(422, map[string]any{"code": "validation_failed"}), nil
		}
		items, err := s.service.List(ctx, input.Site, input.SchemaName, input.Limit)
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
		if err := s.service.Delete(ctx, input.Site, input.SchemaName, input.ID); err != nil {
			return httpJSON(404, map[string]any{"code": "not_found"}), nil
		}
		return httpJSON(200, map[string]any{"deleted": true, "id": input.ID}), nil
	case "admin.surface.get":
		return httpJSON(200, map[string]any{"version": 1, "plugin": name, "requiredCapabilities": []string{"admin.surface.get", "forms.list", "forms.delete"}, "pages": []any{}}), nil
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
