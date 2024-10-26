package otel

import (
	"fmt"
	"os"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"
)

type Exporter string

const (
	zipkinExp Exporter = "zipkin"
	jaegerExp Exporter = "jaeger"
	stdout    Exporter = "stdout"
	stderr    Exporter = "stderr"
	otlp      Exporter = "otlp"
)

type Client string

const (
	grpcClient Client = "grpc"
	httpClient Client = "http"
)

// Resource describes an entity about which identifying information and metadata is exposed.
// Resource is an immutable object, equivalent to a map from key to unique value
type Resource struct {
	ServiceNameKey       string `mapstructure:"service_name"`
	ServiceNamespaceKey  string `mapstructure:"service_namespace"`
	ServiceInstanceIDKey string `mapstructure:"service_instance_id"`
	ServiceVersionKey    string `mapstructure:"service_version"`
}

type Config struct {
	// Resource describes an entity about which identifying information and metadata is exposed.
	Resource *Resource `mapstructure:"resource"`
	// Insecure endpoint (http)
	Insecure bool `mapstructure:"insecure"`
	// Compress - use gzip compression
	Compress bool `mapstructure:"compress"`
	// Exporter type, can be zipkin,stdout or otlp
	Exporter Exporter `mapstructure:"exporter"`
	// CustomURL to use to send spans, has effect only for the HTTP exporter
	CustomURL string `mapstructure:"custom_url"`
	// Client
	Client Client `mapstructure:"client"`
	// Endpoint to connect
	Endpoint string `mapstructure:"endpoint"`
	// ServiceName describes the service in the attributes
	ServiceName string `mapstructure:"service_name"`
	// ServiceVersion in semver format
	ServiceVersion string `mapstructure:"service_version"`
	// Headers for the otlp protocol
	Headers map[string]string `mapstructure:"headers"`
}

func (c *Config) InitDefault(log *zap.Logger) {
	if c.Exporter == "" {
		c.Exporter = otlp
	}

	if c.ServiceName != "" {
		log.Warn("service_name is deprecated, use resource.service_name instead")
	}
	if c.ServiceVersion != "" {
		log.Warn("service_version is deprecated, use resource.service_version instead")
	}
	if c.Exporter == jaegerExp {
		log.Warn("jaeger exporter is deprecated, use OTLP instead: https://github.com/roadrunner-server/roadrunner/issues/1699")
	}

	switch c.Client {
	case grpcClient, httpClient:
		// ok value, do nothing
	case "":
		c.Client = httpClient
		setClientFromEnv(&c.Client, log)
	default:
		log.Warn("unknown exporter client", zap.String("client", string(c.Client)))
		c.Client = httpClient
	}

	if c.Resource == nil {
		c.Resource = &Resource{}
	}

	envAttrs := resource.Environment()
	fillValue(&c.Resource.ServiceNameKey, c.ServiceName, envAttrs, semconv.ServiceNameKey, "RoadRunner")
	fillValue(&c.Resource.ServiceVersionKey, c.ServiceVersion, envAttrs, semconv.ServiceVersionKey, "1.0.0")
	fillValue(&c.Resource.ServiceInstanceIDKey, "", envAttrs, semconv.ServiceInstanceIDKey, uuid.NewString())
	fillValue(&c.Resource.ServiceNamespaceKey, "", envAttrs, semconv.ServiceNamespaceKey, fmt.Sprintf("RoadRunner-%s", uuid.NewString()))
}

func setClientFromEnv(client *Client, log *zap.Logger) {
	// https://opentelemetry.io/docs/specs/otel/protocol/exporter/#specify-protocol
	exporterEnv := "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL"
	exporterVal := os.Getenv(exporterEnv)
	if exporterVal == "" {
		exporterEnv = "OTEL_EXPORTER_OTLP_PROTOCOL"
		exporterVal = os.Getenv(exporterEnv)
	}
	switch exporterVal {
	case "":
		// env var not set, do not change the client
	case "grpc":
		*client = grpcClient
	case "http/protobuf":
		*client = httpClient
	case "http/json":
		log.Warn("unsupported exporter protocol", zap.String("env.name", exporterEnv), zap.String("env.value", exporterVal))
	default:
		log.Warn("unknown exporter protocol", zap.String("env.name", exporterEnv), zap.String("env.value", exporterVal))
	}
}

func fillValue(target *string, fromConf string, fromResource *resource.Resource, fromResourceKey attribute.Key, fromDefault string) {
	if *target != "" {
		return
	}
	if fromConf != "" {
		*target = fromConf
		return
	}
	if resValue, haveValue := fromResource.Set().Value(fromResourceKey); haveValue {
		if resStr := resValue.AsString(); resStr != "" {
			*target = resStr
			return
		}
	}
	*target = fromDefault
}
