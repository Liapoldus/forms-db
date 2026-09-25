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
	"github.com/Liapoldus/forms-db/internal/infrastructure/security"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
	"github.com/Liapoldus/forms-db/internal/presentation/plugin"
	"github.com/Liapoldus/pluginprotocol/transport"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	listener, err := transport.ListenLoopback()
	if err != nil {
		log.Printf("forms-db listen failed: %v", err)
		os.Exit(1)
	}
	defer listener.Close()
	var stop func()
	cursorSigner, _ := security.LoadCursorSigner()
	pluginServer := plugin.NewServerWithRepositoryBuilderAndCursorSigner(
		application.Service{Repository: storage.NewMemoryRepository()}, buildRepository, cursorSigner, func() { stop() },
	)
	server := transport.NewServer(pluginServer, transport.ServerOptions{})
	stop = server.GracefulStop
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		server.GracefulStop()
	case err := <-serveErr:
		if err != nil {
			log.Printf("forms-db server stopped: %v", err)
			os.Exit(1)
		}
	}
}

func buildRepository(ctx context.Context, settings config.Settings) (interfaces.Repository, error) {
	if settings.Driver == "memory" {
		return storage.NewMemoryRepository(), nil
	}
	return storage.NewRepository(ctx, settings.Driver, settings.DSN, settings.TablePrefix, settings.Schemas)
}
