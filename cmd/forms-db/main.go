package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/infrastructure"
	"github.com/Liapoldus/forms-db/internal/protocol"
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
	plugin := protocol.NewServer(application.Service{Repository: infrastructure.NewMemoryRepository()}, func() { stop() })
	server := transport.NewServer(plugin, transport.ServerOptions{})
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
