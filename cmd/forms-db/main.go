package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
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
	pluginServer := plugin.NewServerWithRepositoryBuilder(
		application.Service{Repository: storage.NewMemoryRepository()}, buildRepository, func() { stop() },
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
	switch settings.Driver {
	case "memory":
		return storage.NewMemoryRepository(), nil
	case "sqlite":
		return storage.NewSQLiteRepository(ctx, settings.DSN, settings.TablePrefix)
	default:
		return nil, errors.New("unsupported forms storage driver")
	}
}
