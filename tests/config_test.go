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

func TestConfig_ResourceValuePrecedence(t *testing.T) {
	cases := []struct {
		name        string
		cfg         otel.Config
		env         string
		wantName    string
		wantVersion string
	}{
		{
			name: "resource overrides deprecated fields and environment",
			cfg: otel.Config{
				ServiceName:    "deprecated-name",
				ServiceVersion: "9.9.9",
				Resource:       &otel.Resource{ServiceNameKey: "explicit-name", ServiceVersionKey: "2.0.0"},
			},
			env:         "service.name=env-name,service.version=3.0.0",
			wantName:    "explicit-name",
			wantVersion: "2.0.0",
		},
		{
			name:        "deprecated fields override environment",
			cfg:         otel.Config{ServiceName: "deprecated-name", ServiceVersion: "9.9.9"},
			env:         "service.name=env-name,service.version=3.0.0",
			wantName:    "deprecated-name",
			wantVersion: "9.9.9",
		},
		{
			name:        "environment supplies missing fields",
			env:         "service.name=env-name,service.version=3.0.0",
			wantName:    "env-name",
			wantVersion: "3.0.0",
		},
		{
			name:        "empty environment attributes use defaults",
			env:         "service.name=,service.version=",
			wantName:    "RoadRunner",
			wantVersion: "1.0.0",
		},
		{
			name:        "missing version uses default",
			cfg:         otel.Config{ServiceName: "ignored-deprecated", Resource: &otel.Resource{ServiceNameKey: "explicit-name"}},
			wantName:    "explicit-name",
			wantVersion: "1.0.0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OTEL_SERVICE_NAME", "")
			t.Setenv("OTEL_RESOURCE_ATTRIBUTES", tc.env)
			tc.cfg.InitDefault(discardLogger())
			require.Equal(t, tc.wantName, tc.cfg.Resource.ServiceNameKey)
			require.Equal(t, tc.wantVersion, tc.cfg.Resource.ServiceVersionKey)
		})
	}
}
