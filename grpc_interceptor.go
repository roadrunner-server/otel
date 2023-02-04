package otel

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

// type alias for the interceptors
type grpcInterceptor func() grpc.UnaryServerInterceptor

func GrpcHandler(interceptor grpcInterceptor) grpc.UnaryServerInterceptor {
	return interceptor()
}

func grpcWrapper(prop propagation.TextMapPropagator, tr trace.TracerProvider) grpcInterceptor {
	return func() grpc.UnaryServerInterceptor {
		return otelgrpc.UnaryServerInterceptor(
			otelgrpc.WithTracerProvider(tr),
			otelgrpc.WithPropagators(prop),
		)
	}
}
