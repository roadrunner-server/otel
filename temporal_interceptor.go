package otel

import (
	rrcontext "github.com/roadrunner-server/context"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.temporal.io/sdk/contrib/opentelemetry"
	"go.temporal.io/sdk/interceptor"
)

func newTemporalInterceptor(prop propagation.TextMapPropagator, tr trace.TracerProvider) (interceptor.WorkerInterceptor, error) {
	return opentelemetry.NewTracingInterceptor(
		opentelemetry.TracerOptions{
			Tracer:            tr.Tracer("WorkflowWorker"),
			TextMapPropagator: prop,
			SpanContextKey:    rrcontext.OtelTracerNameKey,
		})
}
