package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Liapoldus/forms-db/tests/fixtures/support"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage/sqlstore"
	"github.com/Liapoldus/forms-db/internal/presentation/restplugin"
	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
)

type secretSource struct{ candidateDSN string }

func (source secretSource) SecretProvider(_ context.Context, reference, purpose string) (sdkmodels.SecretValue, error) {
	if reference != "secret:candidate" || purpose == "" {
		return sdkmodels.SecretValue{}, errors.New("candidate secret unavailable")
	}
	return sdkmodels.NewSecretValue([]byte(source.candidateDSN)), nil
}

func main() {
	if err := run(); err != nil {
		support.Written(fmt.Fprintln(os.Stderr, "SQL reload concurrency scenario failed"))
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	directory, err := os.MkdirTemp("", "forms-sql-reload-")
	if err != nil {
		return errors.New("create SQL reload test directory")
	}
	defer func() { support.Check(os.RemoveAll(directory)) }()
	oldDSN := sqliteDSN(filepath.Join(directory, "old.db"))
	candidateDSN := sqliteDSN(filepath.Join(directory, "candidate.db"))
	oldRepository, err := sqlstore.NewRepository(ctx, "sqlite", oldDSN, "old_", nil)
	if err != nil {
		return errors.New("create old SQL repository")
	}
	candidateReady := make(chan struct{}, 1)
	adapter, err := restplugin.New(application.Service{Repository: oldRepository}, func(buildCtx context.Context, settings config.Settings) (interfaces.Repository, error) {
		repository, err := sqlstore.NewRepository(buildCtx, settings.Driver, string(settings.DSN), settings.TablePrefix, settings.Schemas)
		if err == nil {
			candidateReady <- struct{}{}
		}
		return repository, err
	}, secretSource{candidateDSN: candidateDSN})
	if err != nil {
		support.Check(oldRepository.Close())
		return errors.New("create SQL reload adapter")
	}

	started := make(chan struct{})
	release := make(chan struct{})
	useDone := make(chan error, 1)
	inFlightCallUsedOld := false
	go func() {
		useDone <- adapter.Use(func(service application.Service, _ config.Settings) error {
			close(started)
			<-release
			_, err := service.Submit(ctx, models.Submission{
				ID: "old-generation", Site: "portal", Schema: "contact",
				CreatedAt: "2026-10-04T00:00:00Z", Data: map[string]any{"email": "old@example.test"},
			})
			inFlightCallUsedOld = err == nil
			return err
		})
	}()
	<-started

	raw := []byte(`{"driver":"sqlite","dsn":"secret:candidate","tablePrefix":"candidate_","schemas":{}}`)
	configuration, err := sdkmodels.NewConfiguration("generation-candidate-sql", "1", sdkmodels.Digest(raw), raw)
	if err != nil {
		close(release)
		support.Check(oldRepository.Close())
		return errors.New("construct candidate configuration")
	}
	applyDone := make(chan error, 1)
	go func() { applyDone <- adapter.Apply(ctx, configuration) }()
	select {
	case <-candidateReady:
	case <-time.After(10 * time.Second):
		close(release)
		support.Check(oldRepository.Close())
		return errors.New("candidate SQL repository was not prepared")
	}

	reloadCompletedInFlight := false
	select {
	case err := <-applyDone:
		reloadCompletedInFlight = true
		if err == nil {
			return errors.New("reload replaced repository during an in-flight invocation")
		}
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := wait(useDone); err != nil {
		support.Check(oldRepository.Close())
		return errors.New("in-flight SQL invocation failed")
	}
	if !reloadCompletedInFlight {
		if err := wait(applyDone); err != nil {
			support.Check(oldRepository.Close())
			return errors.New("candidate SQL repository was not activated")
		}
	}

	if !inFlightCallUsedOld {
		return errors.New("in-flight invocation did not complete against the old SQL repository")
	}

	_, oldRepositoryErr := oldRepository.List(ctx, "portal", "contact", nil, nil, 10)
	previousClosed := oldRepositoryErr != nil
	support.Check(oldRepository.Close())
	oldRepositoryAfterSwap, err := sqlstore.NewRepository(ctx, "sqlite", oldDSN, "old_", nil)
	if err != nil {
		return errors.New("reopen previous SQL repository")
	}
	oldRecords, err := oldRepositoryAfterSwap.List(ctx, "portal", "contact", nil, nil, 10)
	support.Check(oldRepositoryAfterSwap.Close())
	if err != nil || len(oldRecords) != 1 || oldRecords[0].ID != "old-generation" {
		return errors.New("old SQL data was not durable across repository replacement")
	}

	candidateCallUsedCandidate := false
	if err := adapter.Use(func(service application.Service, settings config.Settings) error {
		_, err := service.Submit(ctx, models.Submission{
			ID: "sql-candidate-record", Site: "portal", Schema: "contact",
			CreatedAt: "2026-10-04T00:00:01Z", Data: map[string]any{"email": "candidate@example.test"},
		})
		if err != nil {
			return err
		}
		items, err := service.List(ctx, "portal", "contact", nil, nil, 10)
		candidateCallUsedCandidate = err == nil && settings.Driver == "sqlite" && len(items) == 1 && items[0].ID == "sql-candidate-record"
		return err
	}); err != nil || !candidateCallUsedCandidate {
		return errors.New("new generation did not serve from the candidate SQL repository")
	}

	report, err := json.Marshal(map[string]bool{
		"inFlightCallUsedOldSQLRepository":     true,
		"reloadWaitedForInFlightCall":          !reloadCompletedInFlight,
		"candidateGenerationServedAfterReload": candidateCallUsedCandidate,
		"previousSQLRepositoryClosedAfterSwap": previousClosed,
	})
	if err != nil {
		return errors.New("encode SQL reload report")
	}
	_, err = fmt.Println(string(report))
	return err
}

func sqliteDSN(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String() + "?mode=rwc"
}

func wait(result <-chan error) error {
	select {
	case err := <-result:
		return err
	case <-time.After(10 * time.Second):
		return errors.New("SQL reload scenario timed out")
	}
}
