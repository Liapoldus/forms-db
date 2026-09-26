package unit

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
	"github.com/Liapoldus/forms-db/internal/presentation/plugin"
	"github.com/Liapoldus/pluginprotocol/pluginv1"
)

func TestConfigApplyWaitsForCallsUsingTheActiveRepository(t *testing.T) {
	active := &blockingRepository{started: make(chan struct{}), release: make(chan struct{})}
	applyReady := make(chan struct{})
	builder := func(_ context.Context, settings config.Settings) (interfaces.Repository, error) {
		if string(settings.DSN) == "active" {
			return active, nil
		}
		close(applyReady)
		return storage.NewMemoryRepository(), nil
	}
	redeemer := staticGrantRedeemer{configSecrets: map[string][]byte{"active-reference": []byte("active")}}
	server := plugin.NewServerWithRepositoryBuilder(
		application.Service{Repository: storage.NewMemoryRepository()}, builder, nil,
	)
	server.SetGrantBrokerDialer(func(context.Context, *pluginv1.BootstrapRequest) (plugin.GrantRedeemer, error) {
		return redeemer, nil
	})
	bootstrapTestServer(t, server, redeemer)
	applySettings(t, server, `{"driver":"sqlite","dsn":"active-reference","schemas":{"contact":{"type":"object"}}}`)

	callDone := make(chan *pluginv1.CallResponse, 1)
	go func() {
		response, _ := server.Call(context.Background(), &pluginv1.CallRequest{
			Capability: "forms.submit",
			Payload:    []byte(`{"site":"portal","schemaName":"contact","data":{}}`),
		})
		callDone <- response
	}()
	<-active.started

	applyDone := make(chan error, 1)
	go func() {
		_, err := server.ConfigApply(context.Background(), configApplyRequest(`{"driver":"memory","schemas":{"contact":{"type":"object"}}}`))
		applyDone <- err
	}()
	<-applyReady
	select {
	case err := <-applyDone:
		t.Fatalf("ConfigApply returned before the call released its active repository: err=%v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(active.release)
	response := <-callDone
	if response == nil {
		t.Fatal("forms.submit returned no response")
	}
	var envelope struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(response.GetPayload(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Status != 200 {
		t.Fatalf("in-flight call must finish before its repository is retired: status=%d", envelope.Status)
	}
	if err := <-applyDone; err != nil {
		t.Fatal(err)
	}
}

type blockingRepository struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	mu      sync.Mutex
	closed  bool
}

func (r *blockingRepository) Submit(_ context.Context, submission models.Submission) (models.Submission, error) {
	r.once.Do(func() { close(r.started) })
	<-r.release
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return models.Submission{}, errors.New("repository closed during submit")
	}
	submission.ID = "frm_fixture"
	submission.CreatedAt = "2026-01-01T00:00:00Z"
	return submission, nil
}

func (r *blockingRepository) List(context.Context, string, string, *models.SubmissionFilter, *models.SubmissionCursor, int) ([]models.Submission, error) {
	return nil, nil
}

func (r *blockingRepository) Delete(context.Context, string, string, string) error { return nil }

func (r *blockingRepository) Close() error {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	return nil
}
