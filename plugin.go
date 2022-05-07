package otel

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/roadrunner-server/api/v2/plugins/config"
	"github.com/roadrunner-server/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.uber.org/zap"

	// gzip grpc compressor
	_ "google.golang.org/grpc/encoding/gzip"
)

const (
	name string = "otel"
)

type Plugin struct {
	cfg         *Config
	log         *zap.Logger
	tracer      *sdktrace.TracerProvider
	propagators propagation.TextMapPropagator
	mdw         mdw
}

func (p *Plugin) Init(cfg config.Configurer, log *zap.Logger) error {
	const op = errors.Op("otel_plugin_init")

	if !cfg.Has(name) {
		return errors.E(errors.Disabled)
	}

	err := cfg.UnmarshalKey(name, &p.cfg)
	if err != nil {
		return errors.E(op, err)
	}

	// init logger
	p.log = &zap.Logger{}
	*p.log = *log

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

	p.propagators = propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	p.mdw = wrapper(p.propagators, p.tracer, p.cfg.ServiceName)
	otel.SetTracerProvider(p.tracer)

	return nil
}

func (p *Plugin) Middleware(next http.Handler) http.Handler {
	return Handler(next, p.mdw)
}

func (p *Plugin) Serve() chan error {
	return make(chan error, 1)
}

func (p *Plugin) Stop() error {
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

func (p *Plugin) Name() string {
	return name
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
