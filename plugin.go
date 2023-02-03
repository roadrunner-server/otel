package otel

import (
	"context"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/roadrunner-server/errors"
	jprop "go.opentelemetry.io/contrib/propagators/jaeger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"go.uber.org/zap"
	"google.golang.org/grpc"

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
	// Has checks if config section exists.
	Has(name string) bool
}

type Plugin struct {
	cfg         *Config
	log         *zap.Logger
	tracer      *sdktrace.TracerProvider
	propagators propagation.TextMapPropagator
	mdw         mdw
	intcpt      intcpt
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
	p.cfg.InitDefault()

	var exporter sdktrace.SpanExporter
	var client otlptrace.Client

	switch p.cfg.Exporter {
	case stdout:
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint(), stdouttrace.WithWriter(os.Stdout))
		if err != nil {
			return err
		}
	case zipkinExp:
		exporter, err = zipkin.New(p.cfg.Endpoint)
		if err != nil {
			return err
		}
	case jaegerExp:
		exporter, err = jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(p.cfg.Endpoint)))
		if err != nil {
			return err
		}
	case jaegerAgent:
		host, port, errHp := net.SplitHostPort(p.cfg.Endpoint)
		if errHp != nil {
			return errHp
		}

		exporter, err = jaeger.New(jaeger.WithAgentEndpoint(jaeger.WithAgentHost(host), jaeger.WithAgentPort(port)))
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

	p.tracer = sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(newResource(p.cfg.ServiceName, p.cfg.ServiceVersion, cfg.RRVersion())),
	)

	p.propagators = propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}, jprop.Jaeger{})
	p.mdw = httpWrapper(p.propagators, p.tracer, p.cfg.ServiceName)
	p.intcpt = grpcWrapper(p.propagators, p.tracer)
	otel.SetTracerProvider(p.tracer)

	return nil
}

func (p *Plugin) Middleware(next http.Handler) http.Handler {
	return HTTPHandler(next, p.mdw)
}

func (p *Plugin) Interceptor() grpc.UnaryServerInterceptor {
	return GrpcHandler(p.intcpt)
}

func (p *Plugin) Serve() chan error {
	return make(chan error, 1)
}

func (p *Plugin) Stop(context.Context) error {
	// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/sdk.md#forceflush
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	err := p.tracer.ForceFlush(ctx)
	if err != nil {
		return err
	}

	// https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/trace/sdk.md#shutdown
	ctx2, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	err = p.tracer.Shutdown(ctx2)
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

func newResource(serviceName, serviceVersion, rrVersion string) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.OSNameKey.String(runtime.GOOS),
		semconv.ServiceNameKey.String(serviceName),
		semconv.ServiceVersionKey.String(serviceVersion),
		semconv.WebEngineNameKey.String("RoadRunner"),
		semconv.WebEngineVersionKey.String(rrVersion),
		semconv.HostArchKey.String(runtime.GOARCH),
		semconv.TelemetrySDKNameKey.String("opentelemetry"),
		semconv.TelemetrySDKLanguageKey.String("go"),
		semconv.TelemetrySDKVersionKey.String(otel.Version()),
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

	options = append(options, otlptracegrpc.WithEndpoint(cfg.Endpoint))
	options = append(options, otlptracegrpc.WithHeaders(cfg.Headers))

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

	options = append(options, otlptracehttp.WithEndpoint(cfg.Endpoint))
	options = append(options, otlptracehttp.WithHeaders(cfg.Headers))

	return options
}
