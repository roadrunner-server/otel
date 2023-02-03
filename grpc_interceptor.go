package otel

import (
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

// type alias for the interceptors
type intcpt func() grpc.UnaryServerInterceptor

func GrpcHandler(interceptor intcpt) grpc.UnaryServerInterceptor {
	return interceptor()
}

func grpcWrapper(tr trace.TracerProvider, sn string) intcpt {
	return func() grpc.UnaryServerInterceptor {
		return otelgrpc.UnaryServerInterceptor(
			otelgrpc.WithTracerProvider(tr),
			otelgrpc.WithPropagators(propagation.TraceContext{}),
		)
	}
}
