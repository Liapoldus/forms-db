package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/presentation/restplugin"
	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
)

type repository struct {
	name     string
	closed   atomic.Bool
	closeOne sync.Once
	closedCh chan struct{}
}

func newRepository(name string) *repository {
	return &repository{name: name, closedCh: make(chan struct{})}
}

func (store *repository) Submit(_ context.Context, item models.Submission) (models.Submission, error) {
	if store.closed.Load() {
		return models.Submission{}, errors.New("repository is closed")
	}
	return item, nil
}

func (store *repository) List(context.Context, string, string, *models.SubmissionFilter, *models.SubmissionCursor, int) ([]models.Submission, error) {
	if store.closed.Load() {
		return nil, errors.New("repository is closed")
	}
	return []models.Submission{{ID: store.name}}, nil
}

func (store *repository) Delete(context.Context, string, string, string) error {
	if store.closed.Load() {
		return errors.New("repository is closed")
	}
	return nil
}

func (store *repository) Close() error {
	store.closeOne.Do(func() {
		store.closed.Store(true)
		close(store.closedCh)
	})
	return nil
}

func main() {
	old := newRepository("old")
	candidate := newRepository("candidate")
	built := make(chan struct{})
	active, err := restplugin.New(application.Service{Repository: old}, func(context.Context, config.Settings) (interfaces.Repository, error) {
		close(built)
		return candidate, nil
	}, nil)
	if err != nil {
		panic("adapter construction failed")
	}

	raw := []byte(`{"driver":"memory","schemas":{}}`)
	configuration, err := sdkmodels.NewConfiguration("generation-2", "1", sdkmodels.Digest(raw), raw)
	if err != nil {
		panic("candidate configuration construction failed")
	}

	started := make(chan struct{})
	release := make(chan struct{})
	useDone := make(chan error, 1)
	go func() {
		useDone <- active.Use(func(service application.Service, _ config.Settings) error {
			close(started)
			<-release
			items, err := service.List(context.Background(), "portal", "contact", nil, nil, 1)
			if err != nil || len(items) != 1 || items[0].ID != old.name {
				return errors.New("in-flight call lost its original repository")
			}
			return nil
		})
	}()
	<-started

	applyDone := make(chan error, 1)
	go func() { applyDone <- active.Apply(context.Background(), configuration) }()
	<-built

	closedWhileInFlight := false
	applyCompletedWhileInFlight := false
	select {
	case <-old.closedCh:
		closedWhileInFlight = true
	case err := <-applyDone:
		applyCompletedWhileInFlight = true
		if err != nil {
			panic("candidate apply failed")
		}
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := wait(useDone); err != nil {
		panic("in-flight call failed")
	}
	if !applyCompletedWhileInFlight {
		if err := wait(applyDone); err != nil {
			panic("candidate apply failed")
		}
	}

	candidateServes := false
	if err := active.Use(func(service application.Service, _ config.Settings) error {
		items, err := service.List(context.Background(), "portal", "contact", nil, nil, 1)
		candidateServes = err == nil && len(items) == 1 && items[0].ID == candidate.name
		return err
	}); err != nil {
		panic("candidate repository is unavailable after swap")
	}
	_ = candidate.Close()

	report := map[string]bool{
		"oldRepositoryNotClosedWhileInFlight": !closedWhileInFlight,
		"applyWaitedForInvocation":            !applyCompletedWhileInFlight && !closedWhileInFlight,
		"oldRepositoryClosedAfterSwap":        old.closed.Load(),
		"candidateServesAfterSwap":            candidateServes,
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		panic("fixture report encoding failed")
	}
	fmt.Println(string(encoded))
}

func wait(result <-chan error) error {
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		return errors.New("concurrency scenario timed out")
	}
}
