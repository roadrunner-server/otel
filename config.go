package otel

import (
	"fmt"

	"github.com/google/uuid"
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

	if c.ServiceName == "" {
		c.ServiceName = "RoadRunner"
	} else {
		log.Warn("service_name is deprecated, use resource.service_name instead")
	}

	if c.ServiceVersion == "" {
		c.ServiceVersion = "1.0.0"
	} else {
		log.Warn("service_version is deprecated, use resource.service_version instead")
	}

	if c.Exporter == jaegerExp {
		log.Warn("jaeger exporter is deprecated, use OTLP instead: https://github.com/roadrunner-server/roadrunner/issues/1699")
	}

	switch c.Client {
	case grpcClient:
	case httpClient:
	default:
		c.Client = httpClient
	}

	if c.Resource == nil {
		c.Resource = &Resource{
			// https://github.com/open-telemetry/opentelemetry-specification/blob/v1.25.0/specification/resource/semantic_conventions/README.md#service-experimental
			ServiceNameKey:       c.ServiceName,
			ServiceVersionKey:    c.ServiceVersion,
			ServiceInstanceIDKey: uuid.NewString(),
			ServiceNamespaceKey:  fmt.Sprintf("RoadRunner-%s", uuid.NewString()),
		}

		return
	}

	if c.Resource.ServiceNameKey == "" {
		c.Resource.ServiceNameKey = c.ServiceName
	}

	if c.Resource.ServiceVersionKey == "" {
		c.Resource.ServiceVersionKey = c.ServiceVersion
	}

	if c.Resource.ServiceInstanceIDKey == "" {
		c.Resource.ServiceInstanceIDKey = uuid.NewString()
	}

	if c.Resource.ServiceNamespaceKey == "" {
		c.Resource.ServiceNamespaceKey = fmt.Sprintf("RoadRunner-%s", uuid.NewString())
	}
}
