package otel

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/roadrunner-server/api/v2/plugins/config"
	"github.com/roadrunner-server/errors"
	"github.com/roadrunner-server/sdk/v2/utils"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	name string = "otel"
)

type Plugin struct {
	cfg         *Config
	log         *zap.Logger
	once        sync.Once
	tracer      *sdktrace.TracerProvider
	propagators propagation.TextMapPropagator
	handler     http.Handler
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

	p.log = &zap.Logger{}
	*p.log = *log

	p.cfg.InitDefault()
	var exporter sdktrace.SpanExporter
	switch Exporter(p.cfg.Exporter) {
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
		options := make([]otlptracehttp.Option, 0, 5)
		if p.cfg.Insecure {
			options = append(options, otlptracehttp.WithInsecure())
		}
		if p.cfg.Compress {
			options = append(options, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
		}

		if p.cfg.CustomURL != "" {
			options = append(options, otlptracehttp.WithURLPath(p.cfg.CustomURL))
		}

		options = append(options, otlptracehttp.WithEndpoint(p.cfg.Endpoint))
		options = append(options, otlptracehttp.WithHeaders(p.cfg.Headers))
		client := otlptracehttp.NewClient(options...)
		// 1 min timeout
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		exporter, err = otlptrace.New(ctx, client)
		if err != nil {
			return err
		}
	default:
		return errors.Errorf("unknown exporter: %s", p.cfg.Exporter)
	}

	p.tracer = sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(newResource(p.cfg.ServiceName, p.cfg.ServiceVersion)),
	)

	p.propagators = propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	otel.SetTracerProvider(p.tracer)

	return nil
}

func (p *Plugin) Middleware(next http.Handler) http.Handler {
	p.once.Do(func() {
		p.handler = otelhttp.NewHandler(next, "",
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.RequestURI
			}),
			otelhttp.WithSpanOptions(
				trace.WithNewRoot(),
				trace.WithSpanKind(trace.SpanKindServer)),
			otelhttp.WithPropagators(p.propagators),
			otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents))
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), utils.OtelTracerNameKey, p.cfg.ServiceName)
		p.handler.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (p *Plugin) Serve() chan error {
	return make(chan error, 1)
}

func (p *Plugin) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()
	err := p.tracer.ForceFlush(ctx)
	if err != nil {
		return err
	}

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

func newResource(serviceName, serviceVersion string) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.OSNameKey.String(runtime.GOOS),
		semconv.ServiceNameKey.String(serviceName),
		semconv.ServiceVersionKey.String(serviceVersion),
	)
}
