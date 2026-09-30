package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
	"github.com/Liapoldus/forms-db/internal/presentation/plugin"
	transport "github.com/Liapoldus/pluginprotocol/presentation/sdk"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	pluginServer := plugin.NewServerWithRepositoryBuilderAndGrantRedeemer(
		application.Service{Repository: storage.NewMemoryRepository()}, buildRepository, nil, nil,
	)
	if err := transport.ServeInheritedLocalSession(ctx, pluginServer, transport.ServerOptions{}); err != nil {
		log.Printf("forms-db server stopped: %v", err)
		os.Exit(1)
	}
}

func buildRepository(ctx context.Context, settings config.Settings) (interfaces.Repository, error) {
	if settings.Driver == "memory" {
		return storage.NewMemoryRepository(), nil
	}
	return storage.NewRepository(ctx, settings.Driver, string(settings.DSN), settings.TablePrefix, settings.Schemas)
}
