package tests

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/roadrunner-server/otel/v6"
	"github.com/stretchr/testify/require"
)

// mockConfigurer satisfies the otel plugin's Configurer interface. UnmarshalKey
// hands back a pre-built *otel.Config instead of decoding a real config file,
// which keeps these tests focused on the plugin wiring rather than RoadRunner's
// config decoder.
type mockConfigurer struct {
	cfg *otel.Config
}

func (m *mockConfigurer) RRVersion() string { return "2025.1.0" }

func (m *mockConfigurer) Has(string) bool { return m.cfg != nil }

func (m *mockConfigurer) UnmarshalKey(_ string, out any) error {
	p, ok := out.(**otel.Config)
	if !ok {
		return fmt.Errorf("mockConfigurer: unexpected target type %T", out)
	}
	*p = m.cfg
	return nil
}

type mockLogger struct{}

func (mockLogger) NamedLogger(string) *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newConfigurer(cfg *otel.Config) *mockConfigurer { return &mockConfigurer{cfg: cfg} }

// TestPlugin_InitExporterSelection covers the exporter-selection branches of
// Plugin.Init. The stdout/stderr exporters initialize without any network; the
// deprecated and unknown exporters must fail fast with an actionable error.
func TestPlugin_InitExporterSelection(t *testing.T) {
	cases := []struct {
		name    string
		cfg     *otel.Config
		wantErr bool
	}{
		{"stdout", &otel.Config{Exporter: otel.Exporter("stdout")}, false},
		{"stderr", &otel.Config{Exporter: otel.Exporter("stderr")}, false},
		{"jaeger is deprecated", &otel.Config{Exporter: otel.Exporter("jaeger")}, true},
		{"zipkin is deprecated", &otel.Config{Exporter: otel.Exporter("zipkin")}, true},
		{"unknown exporter", &otel.Config{Exporter: otel.Exporter("bogus")}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &otel.Plugin{}
			err := p.Init(newConfigurer(tc.cfg), mockLogger{})
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NoError(t, p.Stop(context.Background()))
		})
	}
}

// TestPlugin_LifecycleAndMiddleware boots the plugin with a stdout exporter and
// checks the full surface a RoadRunner container relies on: the plugin name,
// the tracer and Temporal interceptor are wired, the HTTP middleware forwards
// requests untouched, and Stop flushes without error.
func TestPlugin_LifecycleAndMiddleware(t *testing.T) {
	p := &otel.Plugin{}
	require.NoError(t, p.Init(newConfigurer(&otel.Config{Exporter: otel.Exporter("stdout")}), mockLogger{}))

	require.Equal(t, "otel", p.Name())
	require.NotNil(t, p.Tracer(), "tracer provider must be initialized")
	require.NotNil(t, p.WorkerInterceptor(), "temporal worker interceptor must be initialized")

	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, "ok")
	})

	srv := httptest.NewServer(p.Middleware(next))
	t.Cleanup(srv.Close)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/hello", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })

	require.True(t, called, "the wrapped handler must be invoked")
	require.Equal(t, http.StatusTeapot, resp.StatusCode, "status code must propagate through the middleware")

	require.NoError(t, p.Stop(context.Background()))
}
