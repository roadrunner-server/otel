package otel

import (
	"github.com/roadrunner-server/sdk/v4/utils"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"go.temporal.io/sdk/contrib/opentelemetry"
	"go.temporal.io/sdk/interceptor"
)

// type alias for the interceptors
type temporalInterceptor func() interceptor.Interceptor

func TemporalHandler(interceptor temporalInterceptor) interceptor.Interceptor {
	return interceptor()
}

func temporalWrapper(prop propagation.TextMapPropagator, tr trace.TracerProvider) temporalInterceptor {
	return func() interceptor.Interceptor {
		traceInterceptor, _ := opentelemetry.NewTracingInterceptor(
			opentelemetry.TracerOptions{
				Tracer:            tr.Tracer("WorkflowWorker"),
				TextMapPropagator: prop,
				SpanContextKey:    utils.OtelTracerNameKey,
			})
		return traceInterceptor
	}
}
