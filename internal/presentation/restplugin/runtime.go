package restplugin

import (
	"context"
	"io"
	"log"
	"net/http"
	"time"

	sdkapp "github.com/Liapoldus/plugin-sdk/application"
	sdkinterfaces "github.com/Liapoldus/plugin-sdk/domain/interfaces"
	sdkmodels "github.com/Liapoldus/plugin-sdk/domain/models"
	sdkinfra "github.com/Liapoldus/plugin-sdk/infrastructure"
	sdkpresentation "github.com/Liapoldus/plugin-sdk/presentation"
)

type LifecycleOptions struct {
	Source       sdkinterfaces.ConfigurationSource
	Broker       sdkinterfaces.SecretBroker
	Identity     sdkmodels.ReplicaIdentity
	Credentials  sdkinfra.CredentialsProvider
	CorePeer     sdkmodels.PeerIdentity
	Revocation   sdkinterfaces.RevocationSource
	ErrorLog     *log.Logger
	LogOutput    io.Writer
	AdminSurface sdkpresentation.AdminSurfaceProvider
	AdminActions sdkpresentation.AdminActionHandler
}

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }

func NewHandler(active *Adapter, options LifecycleOptions) (http.Handler, *sdkapp.Lifecycle, *sdkapp.SecretManager, error) {
	if active == nil || options.Source == nil || options.Broker == nil || !options.Identity.Valid() || options.LogOutput == nil {
		return nil, nil, nil, ErrStorageUnavailable
	}
	contract, err := sdkinfra.LoadHTTPContract()
	if err != nil {
		return nil, nil, nil, err
	}
	collector, err := sdkinfra.NewObserverPrometheusCollector(contract)
	if err != nil {
		return nil, nil, nil, err
	}
	logger, err := sdkinfra.NewJSONLogger(contract, options.LogOutput)
	if err != nil {
		return nil, nil, nil, err
	}
	logging, err := sdkapp.NewLoggingObserver(sdkapp.LoggingObserverConfiguration{
		Logger: logger, RedactedKeys: contract.Logging.RedactedKeys,
		AlwaysRedactedKeys:  contract.Logging.AlwaysRedactedKeys,
		RedactedPlaceholder: contract.Logging.RedactedPlaceholder,
		MaximumFields:       contract.Logging.MaximumFields,
		MaximumKeyLength:    contract.Logging.MaximumKeyLength,
		MaximumValueLength:  contract.Logging.MaximumValueLength,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	maximumKindLength := max(len(sdkapp.KindReload), len(sdkapp.KindSecretGrant), len(sdkapp.KindSecretRedemption))
	recorder, err := sdkapp.NewRecorder(sdkapp.RecorderConfiguration{
		Sink:              collectorSink{collector},
		AllowedKinds:      []sdkapp.Kind{sdkapp.KindReload, sdkapp.KindSecretGrant, sdkapp.KindSecretRedemption},
		MaximumKindLength: maximumKindLength,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	observer := sdkapp.Observers{recorder, logging}
	lifecycle, err := sdkapp.NewLifecycle(sdkapp.LifecycleConfiguration{
		Source: options.Source, Applier: active, Identity: options.Identity, Observer: observer,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	secrets, err := sdkapp.NewSecretManager(sdkapp.SecretManagerConfiguration{
		Broker: options.Broker, Clock: wallClock{}, Lifecycle: lifecycle,
		Observer: observer, MaximumTrackedGrants: 1024,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	active.SetSecretProvider(secrets)
	handler, err := sdkpresentation.NewHandlerSet(sdkpresentation.HandlerConfiguration{
		Contracts: presentationContracts(contract), Lifecycle: lifecycle,
		Readiness:    sdkpresentation.WithoutContext(lifecycle.Readiness),
		Registration: lifecycle, Metadata: active, Metrics: collector,
		AdminSurface: options.AdminSurface, AdminActions: options.AdminActions,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return handler.Handler(), lifecycle, secrets, nil
}

func NewMutualTLSServer(active *Adapter, options LifecycleOptions) (*sdkinfra.MutualTLSServer, *sdkapp.Lifecycle, *sdkapp.SecretManager, error) {
	handler, lifecycle, secrets, err := NewHandler(active, options)
	if err != nil {
		return nil, nil, nil, err
	}
	contract, err := sdkinfra.LoadHTTPContract()
	if err != nil {
		return nil, nil, nil, err
	}
	if options.Credentials == nil || !options.CorePeer.Valid() || options.Revocation == nil {
		return nil, nil, nil, sdkinfra.ErrInvalidServerTLS
	}
	server, err := sdkinfra.NewMutualTLSServer(contract, sdkinfra.MutualTLSServerConfig{
		Handler: handler, Provider: options.Credentials, Peer: options.CorePeer,
		Revocation: options.Revocation, ErrorLog: options.ErrorLog,
	})
	return server, lifecycle, secrets, err
}

type collectorSink struct {
	collector *sdkinfra.ObserverPrometheusCollector
}

func (sink collectorSink) Lifecycle(kind string, outcome sdkmodels.Outcome) {
	sink.collector.Observe(context.Background(), kind, outcome)
}
func (sink collectorSink) PullFailure(sdkmodels.Outcome) { sink.collector.RecordConfigPullFailure() }
func (sink collectorSink) SetReady(ready bool)           { sink.collector.SetReady(ready) }

func presentationContracts(contract sdkinfra.HTTPContract) sdkpresentation.Contracts {
	endpoint := func(name string) sdkpresentation.Endpoint {
		value, err := contract.Endpoint(name)
		if err != nil {
			panic("validated SDK contract endpoint missing")
		}
		return sdkpresentation.Endpoint{Method: value.Method, Path: value.Path}
	}
	document := func(value sdkinfra.DocumentContract) sdkpresentation.DocumentContract {
		return sdkpresentation.DocumentContract{MediaType: value.MediaType, MaximumBytes: value.MaximumBytes,
			Required: append([]string(nil), value.Required...), DigestAlgorithm: value.DigestAlgorithm}
	}
	artifact := contract.Plugin.ArtifactStream
	admin := contract.Plugin.AdminAction
	invocation := admin.InvocationContext
	problems := make(map[string]sdkpresentation.Problem, len(contract.Problems))
	for name, value := range contract.Problems {
		problems[name] = sdkpresentation.Problem{Status: value.Status, Code: value.Code}
	}
	errors := make(map[string]sdkpresentation.Problem, len(contract.Errors))
	for name, value := range contract.Errors {
		errors[name] = sdkpresentation.Problem{Status: value.Status, Code: value.Code}
	}
	return sdkpresentation.Contracts{
		ContractVersion:  contract.ContractVersion,
		IdentityEndpoint: endpoint("identity"), ManifestEndpoint: endpoint("manifest"),
		ConfigSchemaEndpoint: endpoint("configSchema"), HealthEndpoint: endpoint("health"),
		ReadyEndpoint: endpoint("ready"), ReloadEndpoint: endpoint("reload"),
		ArtifactStreamEndpoint: endpoint("artifactStream"),
		AdminSurfaceEndpoint:   endpoint("adminSurface"), AdminActionEndpoint: endpoint("adminAction"),
		MetricsEndpoint: endpoint("metrics"),
		HealthStatus:    contract.Plugin.Responses.Health.Status,
		HealthBody:      cloneStringMap(contract.Plugin.Responses.Health.Body),
		ContentTypes: sdkpresentation.ContentTypes{
			JSON:    contract.Plugin.Responses.ContentTypes.JSON,
			Metrics: contract.Plugin.Responses.ContentTypes.Metrics,
		},
		ReloadRequest:         document(contract.Plugin.ReloadRequest),
		ReloadAcknowledgement: document(contract.Plugin.ReloadAcknowledgement),
		Readiness:             document(contract.Plugin.Readiness),
		Manifest:              document(contract.Plugin.Manifest),
		ConfigurationSchema:   document(contract.Plugin.ConfigurationSchema),
		Registration: document(sdkinfra.DocumentContract{
			MediaType:    contract.Identity.Registration.MediaType,
			MaximumBytes: contract.Identity.Registration.MaximumBytes,
			Required:     contract.Identity.Registration.Required,
		}),
		MaximumMetadataBytes: contract.Plugin.MaximumMetadataBytes,
		ArtifactStream: sdkpresentation.ArtifactStreamContract{
			MediaType: artifact.MediaType, MetadataMediaType: artifact.MetadataMediaType,
			Parts: append([]string(nil), artifact.Parts...), PartOrder: append([]string(nil), artifact.PartOrder...),
			MaximumArtifactBytes: artifact.MaximumArtifactBytes, MinimumArtifactBytes: artifact.MinimumArtifactBytes,
			MaximumMetadataBytes: artifact.MaximumMetadataBytes, MaximumMultipartOverheadBytes: artifact.MaximumMultipartOverheadBytes,
			MaximumRequestBytes: artifact.MaximumRequestBytes, MaximumReceiptBytes: artifact.MaximumReceiptBytes,
			AcceptedStatus: artifact.AcceptedStatus, FilenameForwarded: artifact.FilenameForwarded,
			Deadline: time.Duration(contract.Deadlines.ArtifactStreamSeconds) * time.Second,
			InvocationContext: sdkpresentation.ArtifactInvocationContract{
				MaximumBytes: artifact.InvocationContext.MaximumBytes,
				Required:     append([]string(nil), artifact.InvocationContext.Required...),
				Optional:     append([]string(nil), artifact.InvocationContext.Optional...),
				Headers:      cloneStringMap(artifact.InvocationContext.Headers),
			},
		},
		AdminSurface: document(contract.Plugin.AdminSurface),
		AdminAction: sdkpresentation.AdminActionContract{
			MediaType: admin.MediaType, MaximumRequestBytes: admin.MaximumRequestBytes,
			MaximumResponseBytes: admin.MaximumResponseBytes, MaximumPageIDBytes: admin.MaximumPageIDBytes,
			MaximumActionIDBytes: admin.MaximumActionIDBytes, PathSegmentPattern: admin.PathSegmentPattern,
			ResponseStatus: sdkpresentation.StatusRangeContract{Minimum: admin.ResponseStatus.Minimum, Maximum: admin.ResponseStatus.Maximum},
			Deadline:       time.Duration(admin.DeadlineSeconds) * time.Second,
			InvocationContext: sdkpresentation.AdminInvocationContract{
				MaximumBytes: invocation.MaximumBytes, UnknownHeaderPrefix: invocation.UnknownHeaderPrefix,
				Required: append([]string(nil), invocation.Required...), Optional: append([]string(nil), invocation.Optional...),
				Headers: cloneStringMap(invocation.Headers),
			},
		},
		ReadinessDeadline: time.Duration(contract.Deadlines.PluginReadinessSeconds) * time.Second,
		Problems:          problems, Errors: errors,
		OutcomeProblems: cloneStringMap(contract.OutcomeProblems),
		SuccessOutcomes: append([]string(nil), contract.SuccessOutcomes...),
	}
}

func cloneStringMap(source map[string]string) map[string]string {
	copy := make(map[string]string, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

var _ sdkinterfaces.PluginMetadata = (*Adapter)(nil)
var _ sdkinterfaces.ConfigurationApplier = (*Adapter)(nil)
