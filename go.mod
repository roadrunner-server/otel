module github.com/roadrunner-server/otel/v3

go 1.19

require (
	github.com/roadrunner-server/errors v1.2.0
	github.com/roadrunner-server/sdk/v3 v3.0.0-beta.3
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.36.1
	go.opentelemetry.io/contrib/propagators/jaeger v1.10.0
	go.opentelemetry.io/otel v1.11.0
	go.opentelemetry.io/otel/exporters/jaeger v1.10.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.10.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.10.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.10.0
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.11.0
	go.opentelemetry.io/otel/exporters/zipkin v1.10.0
	go.opentelemetry.io/otel/sdk v1.11.0
	go.opentelemetry.io/otel/trace v1.11.0
	go.uber.org/zap v1.23.0
	google.golang.org/grpc v1.50.0
)

require (
	github.com/cenkalti/backoff/v4 v4.1.3 // indirect
	github.com/felixge/httpsnoop v1.0.3 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.11.3 // indirect
	github.com/openzipkin/zipkin-go v0.4.0 // indirect
	github.com/roadrunner-server/tcplisten v1.2.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.10.0 // indirect
	go.opentelemetry.io/otel/metric v0.32.1 // indirect
	go.opentelemetry.io/proto/otlp v0.19.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	golang.org/x/net v0.0.0-20221004154528-8021a29435af // indirect
	golang.org/x/sys v0.0.0-20221006211917-84dc82d7e875 // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/genproto v0.0.0-20221010155953-15ba04fc1c0e // indirect
	google.golang.org/protobuf v1.28.1 // indirect
)
