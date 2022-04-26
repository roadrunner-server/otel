package otel

type Config struct {
	Insecure       bool              `mapstructure:"insecure"`
	Compress       bool              `mapstructure:"compress"`
	Exporter       string            `mapstructure:"exporter"`
	CustomURL      string            `mapstructure:"custom_url"`
	Endpoint       string            `mapstructure:"endpoint"`
	Operation      string            `mapstructure:"operation"`
	ServiceName    string            `mapstructure:"service_name"`
	ServiceVersion string            `mapstructure:"service_version"`
	Headers        map[string]string `mapstructure:"headers"`
}

func (c *Config) InitDefault() {
	if c.Operation == "" {
		c.Operation = "RR_HANDLER"
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
}
