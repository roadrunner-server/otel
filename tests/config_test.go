package tests

import (
	"io"
	"log/slog"
	"testing"

	"github.com/roadrunner-server/otel/v6"
	"github.com/stretchr/testify/require"
)

// discardLogger returns a slog logger that drops everything; the otel config
// helpers only use it for deprecation warnings which are irrelevant here.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestConfig_Defaults verifies InitDefault fills the documented defaults: the
// OTLP exporter, the HTTP client, and a fully-populated Resource.
func TestConfig_Defaults(t *testing.T) {
	cfg := &otel.Config{}
	cfg.InitDefault(discardLogger())

	require.Equal(t, otel.Exporter("otlp"), cfg.Exporter, "exporter must default to otlp")
	require.Equal(t, otel.Client("http"), cfg.Client, "client must default to http")

	require.NotNil(t, cfg.Resource)
	require.Equal(t, "RoadRunner", cfg.Resource.ServiceNameKey)
	require.Equal(t, "1.0.0", cfg.Resource.ServiceVersionKey)
	require.NotEmpty(t, cfg.Resource.ServiceInstanceIDKey, "instance id must be generated")
	require.NotEmpty(t, cfg.Resource.ServiceNamespaceKey, "namespace must be generated")
}

// TestConfig_ClientSelectionFromEnv verifies the OTEL protocol environment
// variables select the exporter client when none is configured, and that the
// traces-specific variable takes precedence over the generic one.
func TestConfig_ClientSelectionFromEnv(t *testing.T) {
	cases := []struct {
		name    string
		traces  string // OTEL_EXPORTER_OTLP_TRACES_PROTOCOL
		generic string // OTEL_EXPORTER_OTLP_PROTOCOL
		want    otel.Client
	}{
		{"traces protocol wins over generic", "grpc", "http/protobuf", otel.Client("grpc")},
		{"generic http fallback", "", "http/protobuf", otel.Client("http")},
		{"generic grpc fallback", "", "grpc", otel.Client("grpc")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", tc.traces)
			t.Setenv("OTEL_EXPORTER_OTLP_PROTOCOL", tc.generic)

			cfg := &otel.Config{}
			cfg.InitDefault(discardLogger())
			require.Equal(t, tc.want, cfg.Client)
		})
	}
}

// TestConfig_ResourceValuePrecedence verifies the value precedence in the
// resource attributes: an explicit Resource value wins over the deprecated
// top-level field, which in turn wins over the built-in default.
func TestConfig_ResourceValuePrecedence(t *testing.T) {
	// Deprecated top-level fields flow into the Resource when nothing else set them.
	deprecated := &otel.Config{
		ServiceName:    "from-deprecated-name",
		ServiceVersion: "9.9.9",
	}
	deprecated.InitDefault(discardLogger())
	require.Equal(t, "from-deprecated-name", deprecated.Resource.ServiceNameKey)
	require.Equal(t, "9.9.9", deprecated.Resource.ServiceVersionKey)

	// An explicit Resource value takes precedence over the deprecated field,
	// while an unset sibling still falls back to the default.
	explicit := &otel.Config{
		ServiceName: "ignored-deprecated",
		Resource:    &otel.Resource{ServiceNameKey: "explicit-name"},
	}
	explicit.InitDefault(discardLogger())
	require.Equal(t, "explicit-name", explicit.Resource.ServiceNameKey, "explicit resource value must win")
	require.Equal(t, "1.0.0", explicit.Resource.ServiceVersionKey, "unset version must fall back to default")
}
