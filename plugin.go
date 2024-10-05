package otel

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/roadrunner-server/errors"
	jprop "go.opentelemetry.io/contrib/propagators/jaeger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.temporal.io/sdk/interceptor"
	"go.uber.org/zap"

	// gzip grpc compressor
	_ "google.golang.org/grpc/encoding/gzip"
)

const (
	pluginName string = "otel"
)

type Logger interface {
	NamedLogger(name string) *zap.Logger
}

type Configurer interface {
	// RRVersion returns running RR version
	RRVersion() string
	// UnmarshalKey takes a single key and unmarshal it into a Struct.
	UnmarshalKey(name string, out any) error
	// Has checks if a config section exists.
	Has(name string) bool
}

type Plugin struct {
	cfg                 *Config
	log                 *zap.Logger
	tracer              *sdktrace.TracerProvider
	propagators         propagation.TextMapPropagator
	httpMiddleware      httpMiddleware
	temporalInterceptor temporalInterceptor
}

func (p *Plugin) Init(cfg Configurer, log Logger) error { //nolint:gocyclo
	const op = errors.Op("otel_plugin_init")

	if !cfg.Has(pluginName) {
		return errors.E(errors.Disabled)
	}

	err := cfg.UnmarshalKey(pluginName, &p.cfg)
	if err != nil {
		return errors.E(op, err)
	}

	// init logger
	p.log = log.NamedLogger(pluginName)

	// init default configuration
	p.cfg.InitDefault(p.log)

	var exporter sdktrace.SpanExporter
	var client otlptrace.Client

	switch p.cfg.Exporter {
	case stdout:
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint(), stdouttrace.WithWriter(os.Stdout))
		if err != nil {
			return err
		}
	case jaegerExp:
		return errors.Errorf("jaeger exporter is deprecated, use OTLP instead: https://github.com/roadrunner-server/roadrunner/issues/1699")
	case stderr:
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint(), stdouttrace.WithWriter(os.Stderr))
		if err != nil {
			return err
		}
	case zipkinExp:
		exporter, err = zipkin.New(p.cfg.Endpoint)
		if err != nil {
			return err
		}
	case otlp:
		switch p.cfg.Client {
		case httpClient:
			client = otlptracehttp.NewClient(httpOptions(p.cfg)...)
		case grpcClient:
			client = otlptracegrpc.NewClient(grpcOptions(p.cfg)...)
		default:
			return errors.Errorf("unknown client: %s", p.cfg.Client)
		}

		// 1 min timeout
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		exporter, err = otlptrace.New(ctx, client)
		if err != nil {
			return errors.E(op, err)
		}
	default:
		return errors.Errorf("unknown exporter: %s", p.cfg.Exporter)
	}

	resource, err := newResource(p.cfg.Resource, cfg.RRVersion())
	if err != nil {
		return errors.E(op, err)
	}
	p.tracer = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource),
	)

	p.propagators = propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}, jprop.Jaeger{})
	p.httpMiddleware = httpWrapper(p.propagators, p.tracer, p.cfg.ServiceName)
	p.temporalInterceptor = temporalWrapper(p.propagators, p.tracer)
	otel.SetTracerProvider(p.tracer)

	return nil
}

func (p *Plugin) Middleware(next http.Handler) http.Handler {
	return HTTPHandler(next, p.httpMiddleware)
}

func (p *Plugin) WorkerInterceptor() interceptor.WorkerInterceptor {
	return TemporalHandler(p.temporalInterceptor)
}

func (p *Plugin) Serve() chan error {
	return make(chan error, 1)
}

func (p *Plugin) Stop(ctx context.Context) error {
	// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/sdk.md#forceflush
	err := p.tracer.ForceFlush(ctx)
	if err != nil {
		return err
	}

	// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/sdk.md#shutdown
	err = p.tracer.Shutdown(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (p *Plugin) Tracer() *sdktrace.TracerProvider {
	return p.tracer
}

func (p *Plugin) Name() string {
	return pluginName
}

func newResource(res *Resource, rrVersion string) (*resource.Resource, error) {
	return resource.New(context.Background(),
		resource.WithSchemaURL(semconv.SchemaURL),
		resource.WithFromEnv(),
		resource.WithAttributes(
			semconv.OSNameKey.String(runtime.GOOS),
			semconv.ServiceNameKey.String(res.ServiceNameKey),
			semconv.ServiceVersionKey.String(res.ServiceVersionKey),
			semconv.ServiceInstanceIDKey.String(res.ServiceInstanceIDKey),
			semconv.ServiceNamespaceKey.String(res.ServiceNamespaceKey),
			semconv.WebEngineNameKey.String("RoadRunner"),
			semconv.WebEngineVersionKey.String(rrVersion),
			semconv.HostArchKey.String(runtime.GOARCH),
		),
		resource.WithTelemetrySDK(),
	)
}

func grpcOptions(cfg *Config) []otlptracegrpc.Option {
	options := make([]otlptracegrpc.Option, 0, 5)
	if cfg.Insecure {
		options = append(options, otlptracegrpc.WithInsecure())
	}
	if cfg.Compress {
		options = append(options, otlptracegrpc.WithCompressor("gzip"))
	}

	// if unset, OTEL will use the default one automatically
	if cfg.Endpoint != "" {
		options = append(options, otlptracegrpc.WithEndpoint(cfg.Endpoint))
	}

	if len(cfg.Headers) > 0 {
		options = append(options, otlptracegrpc.WithHeaders(cfg.Headers))
	}

	return options
}

func httpOptions(cfg *Config) []otlptracehttp.Option {
	options := make([]otlptracehttp.Option, 0, 5)
	if cfg.Insecure {
		options = append(options, otlptracehttp.WithInsecure())
	}
	if cfg.Compress {
		options = append(options, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
	}

	if cfg.CustomURL != "" {
		options = append(options, otlptracehttp.WithURLPath(cfg.CustomURL))
	}

	// if unset, OTEL will use the default one automatically
	if cfg.Endpoint != "" {
		options = append(options, otlptracehttp.WithEndpoint(cfg.Endpoint))
	}

	if len(cfg.Headers) > 0 {
		options = append(options, otlptracehttp.WithHeaders(cfg.Headers))
	}

	return options
}
