package otel

import (
	"context"
	"net/http"

	"github.com/roadrunner-server/api/v2/plugins/config"
	"github.com/roadrunner-server/errors"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
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
	cfg    *Config
	log    *zap.Logger
	tracer trace.Tracer
	client otlptrace.Client
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

	p.client = otlptracehttp.NewClient()
	p.tracer = otel.GetTracerProvider().Tracer("foo", trace.WithInstrumentationVersion("v0.1.0"), trace.WithSchemaURL(semconv.SchemaURL))
	exporter, err := otlptrace.New(context.Background(), p.client)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(newResource()),
	)

	otel.SetTracerProvider(tracerProvider)

	return nil
}

func (p *Plugin) Middleware(next http.Handler) http.Handler {
	//ctx, span := tracer.Start()
	return otelhttp.NewHandler(next, "rr-request")
}

func newResource() *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceNameKey.String("otlptrace-example"),
		semconv.ServiceVersionKey.String("0.0.1"),
	)
}