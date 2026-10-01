package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Liapoldus/forms-db/internal/application"
	"github.com/Liapoldus/forms-db/internal/domain/interfaces"
	"github.com/Liapoldus/forms-db/internal/infrastructure/config"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage"
	"github.com/Liapoldus/forms-db/internal/presentation/peerplugin"
	"github.com/Liapoldus/forms-db/internal/presentation/restplugin"
	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	sdkinfra "github.com/Liapoldus/plugin-sdk/infrastructure"
	"github.com/Liapoldus/pluginprotocol/presentation/peer"
)

type options struct {
	instanceID, replicaID, restListen, coreURL, coreServerName      string
	coreName, coreURI, caFile, serverCert, serverKey                string
	coreClientName, coreClientURI                                   string
	clientCert, clientKey, crlFile                                  string
	peerListen, peerIdentity, peerCaller, peerCA, peerCert, peerKey string
	peerCarrier                                                     string
}

func main() {
	if run(os.Args[1:]) != nil {
		os.Exit(1)
	}
}

func run(args []string) (runErr error) {
	stage := "bootstrap"
	defer func() {
		if runErr != nil {
			_, _ = io.WriteString(os.Stderr, "forms-db failed at "+stage+"\n")
		}
	}()
	settings, err := parseOptions(args)
	if err != nil {
		return err
	}
	stage = "sdk-contract"
	contract, err := sdkinfra.LoadHTTPContract()
	if err != nil {
		return err
	}
	stage = "credentials"
	credentials, err := sdkinfra.NewFileCredentialsProvider(contract, sdkinfra.CredentialsMaterial{
		CAFile:                settings.caFile,
		ServerCertificateFile: settings.serverCert, ServerKeyFile: settings.serverKey,
		ClientCertificateFile: settings.clientCert, ClientKeyFile: settings.clientKey,
	})
	if err != nil {
		return err
	}
	stage = "credentials-load"
	material, err := credentials.Credentials()
	if err != nil {
		return err
	}
	stage = "revocation"
	revocation, err := sdkinfra.NewRevocation(sdkinfra.RevocationConfiguration{
		Authorities: material.TrustAuthorities(), Files: []string{settings.crlFile},
	})
	if err != nil {
		return err
	}
	stage = "core-identities"
	corePeer, err := sdkmodels.NewPeerIdentity(settings.coreName, settings.coreURI)
	if err != nil {
		return err
	}
	coreClientPeer, err := sdkmodels.NewPeerIdentity(settings.coreClientName, settings.coreClientURI)
	if err != nil {
		return err
	}
	identity, err := sdkmodels.NewReplicaIdentity(settings.instanceID, settings.replicaID)
	if err != nil {
		return err
	}
	stage = "control-client"
	controlClient, err := sdkinfra.NewMutualTLSClient(contract, credentials, sdkinfra.MutualTLSClientConfig{
		Peer: corePeer, ServerName: settings.coreServerName, Revocation: revocation,
	})
	if err != nil {
		return err
	}
	defer controlClient.CloseIdleConnections()
	stage = "config-source"
	source, err := sdkinfra.NewCoreConfigurationSource(contract, settings.coreURL, controlClient)
	if err != nil {
		return err
	}
	stage = "secret-broker"
	broker, err := sdkinfra.NewCoreSecretBroker(contract, settings.coreURL, controlClient, 1024)
	if err != nil {
		return err
	}
	stage = "product-runtime"
	active, err := restplugin.New(application.Service{Repository: storage.NewMemoryRepository()}, buildRepository, nil)
	if err != nil {
		return err
	}
	stage = "sdk-rest-server"
	server, _, secrets, err := restplugin.NewMutualTLSServer(active, restplugin.LifecycleOptions{
		Source: source, Broker: broker, Identity: identity, Credentials: credentials,
		CorePeer: coreClientPeer, Revocation: revocation,
		ErrorLog: log.New(io.Discard, "", 0), LogOutput: os.Stdout,
	})
	if err != nil {
		return err
	}
	stage = "peer-handler"
	peerHandler, err := peerplugin.New(active, secrets, callerAuthorizer{uri: settings.peerCaller})
	if err != nil {
		return err
	}
	stage = "peer-credentials"
	peerCertificate, err := tls.LoadX509KeyPair(settings.peerCert, settings.peerKey)
	if err != nil {
		return err
	}
	peerRootsPEM, err := os.ReadFile(settings.peerCA)
	if err != nil {
		return err
	}
	peerRoots := x509.NewCertPool()
	if !peerRoots.AppendCertsFromPEM(peerRootsPEM) {
		return errors.New("invalid peer trust roots")
	}
	stage = "peer-listener"
	peerServer, err := peer.Listen(peer.ServerConfig{
		Network:  peer.NetworkConfig{Carrier: peer.Carrier(settings.peerCarrier), Endpoint: settings.peerListen},
		Security: peer.SecurityConfig{Identity: settings.peerIdentity, Certificate: peerCertificate, Roots: peerRoots},
		Handler:  peerHandler,
	})
	if err != nil {
		return err
	}
	defer peerServer.Close()
	stage = "serve"
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	serveResult := make(chan error, 2)
	go func() { serveResult <- server.ListenAndServe(settings.restListen) }()
	go func() { serveResult <- peerServer.Sessions(ctx) }()
	select {
	case <-ctx.Done():
	case err := <-serveResult:
		if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, context.Canceled) {
			return err
		}
	}
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	return server.GracefulShutdown(shutdownContext)
}

type callerAuthorizer struct{ uri string }

func (authorizer callerAuthorizer) AuthorizeCall(from peer.PeerIdentity, _ peer.Method) error {
	if from.URI != authorizer.uri {
		return peer.ErrUnauthorized
	}
	return nil
}

func (callerAuthorizer) AuthorizeStream(peer.PeerIdentity, peer.Method) error {
	return peer.ErrUnauthorized
}

func buildRepository(ctx context.Context, settings config.Settings) (interfaces.Repository, error) {
	if settings.Driver == "memory" {
		return storage.NewMemoryRepository(), nil
	}
	return storage.NewRepository(ctx, settings.Driver, string(settings.DSN), settings.TablePrefix, settings.Schemas)
}

func parseOptions(args []string) (options, error) {
	var value options
	flags := flag.NewFlagSet("forms-db", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&value.instanceID, "instance-id", "", "")
	flags.StringVar(&value.replicaID, "replica-id", "", "")
	flags.StringVar(&value.restListen, "rest-listen", "", "")
	flags.StringVar(&value.coreURL, "core-url", "", "")
	flags.StringVar(&value.coreServerName, "core-server-name", "", "")
	flags.StringVar(&value.coreName, "core-common-name", "", "")
	flags.StringVar(&value.coreURI, "core-uri", "", "")
	flags.StringVar(&value.coreClientName, "core-client-common-name", "", "")
	flags.StringVar(&value.coreClientURI, "core-client-uri", "", "")
	flags.StringVar(&value.caFile, "ca-file", "", "")
	flags.StringVar(&value.serverCert, "server-cert", "", "")
	flags.StringVar(&value.serverKey, "server-key", "", "")
	flags.StringVar(&value.clientCert, "client-cert", "", "")
	flags.StringVar(&value.clientKey, "client-key", "", "")
	flags.StringVar(&value.crlFile, "crl-file", "", "")
	flags.StringVar(&value.peerListen, "peer-listen", "", "")
	flags.StringVar(&value.peerIdentity, "peer-identity", "", "")
	flags.StringVar(&value.peerCaller, "peer-allowed-caller", "", "")
	flags.StringVar(&value.peerCA, "peer-ca-file", "", "")
	flags.StringVar(&value.peerCert, "peer-cert", "", "")
	flags.StringVar(&value.peerKey, "peer-key", "", "")
	flags.StringVar(&value.peerCarrier, "peer-carrier", "tcp", "")
	if flags.Parse(args) != nil || flags.NArg() != 0 ||
		value.instanceID == "" || value.replicaID == "" || value.restListen == "" ||
		value.coreURL == "" || value.coreServerName == "" || value.coreName == "" || value.coreClientName == "" ||
		value.peerListen == "" || value.peerIdentity == "" || value.peerCaller == "" ||
		(value.peerCarrier != "tcp" && value.peerCarrier != "quic") ||
		!absoluteFiles(value.caFile, value.serverCert, value.serverKey, value.clientCert, value.clientKey, value.crlFile,
			value.peerCA, value.peerCert, value.peerKey) {
		return options{}, errors.New("invalid forms-db bootstrap options")
	}
	return value, nil
}

func absoluteFiles(paths ...string) bool {
	for _, path := range paths {
		if path == "" || !filepath.IsAbs(path) {
			return false
		}
	}
	return true
}
