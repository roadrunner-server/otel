package otel

type Exporter string

const (
	zipkinExp Exporter = "zipkin"
	stdout    Exporter = "stdout"
	otlp      Exporter = "otlp"
)

type Client string

const (
	grpcClient Client = "grpc"
	httpClient Client = "http"
)

type Config struct {
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

func (c *Config) InitDefault() {
	if c.Exporter == "" {
		c.Exporter = otlp
	}

	if c.ServiceName == "" {
		c.ServiceName = "RoadRunner"
	}

	if c.ServiceVersion == "" {
		c.ServiceVersion = "1.0.0"
	}

	if c.Endpoint == "" {
		// otlp default
		c.Endpoint = "localhost:4318"
	}

	switch c.Client {
	case grpcClient:
	case httpClient:
	default:
		c.Client = httpClient
	}
}
