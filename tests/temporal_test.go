package tests

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.temporal.io/sdk/client"
	otelinterceptor "go.temporal.io/sdk/contrib/opentelemetry"
	"go.temporal.io/sdk/interceptor"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

const otelTestTaskQueue = "otel-interceptor-test"

// EchoActivity upper-cases its input. It runs inside the Go-SDK worker, so no
// PHP worker is required to exercise the Temporal integration.
func EchoActivity(_ context.Context, in string) (string, error) {
	return strings.ToUpper(in), nil
}

// EchoWorkflow runs EchoActivity and returns its result.
func EchoWorkflow(ctx workflow.Context, in string) (string, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Second,
	})

	var out string
	if err := workflow.ExecuteActivity(ctx, EchoActivity, in).Get(ctx, &out); err != nil {
		return "", err
	}
	return out, nil
}

// TestTemporalOtelInterceptor_DevServer runs a real Temporal workflow against a
// `temporal server start-dev` instance through the OpenTelemetry worker
// interceptor the otel plugin builds (go.temporal.io/sdk/contrib/opentelemetry),
// and asserts the produced spans.
//
// It follows the http plugin's otel test approach: an in-memory exporter behind
// a synchronous TracerProvider, read back via GetSpans() — no os.Stdout/os.Stderr
// redirection. The test is skipped unless TEMPORAL_ADDRESS points at a running
// dev server (CI sets it to 127.0.0.1:7233).
func TestTemporalOtelInterceptor_DevServer(t *testing.T) {
	addr := os.Getenv("TEMPORAL_ADDRESS")
	if addr == "" {
		t.Skip("TEMPORAL_ADDRESS not set; run `temporal server start-dev` and set TEMPORAL_ADDRESS=127.0.0.1:7233 to run this test")
	}

	// In-memory exporter with a synchronous syncer: spans are exported the moment
	// they end, so they are available right after the worker drains.
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	// Built exactly as the otel plugin builds its Temporal interceptor, but over
	// the in-memory tracer so the spans can be asserted directly.
	ti, err := otelinterceptor.NewTracingInterceptor(otelinterceptor.TracerOptions{
		Tracer: tp.Tracer("WorkflowWorker"),
	})
	require.NoError(t, err)

	c, err := client.Dial(client.Options{HostPort: addr, Namespace: "default"})
	require.NoError(t, err, "dial temporal dev server at %s", addr)
	defer c.Close()

	w := worker.New(c, otelTestTaskQueue, worker.Options{
		Interceptors: []interceptor.WorkerInterceptor{ti},
	})
	w.RegisterWorkflow(EchoWorkflow)
	w.RegisterActivity(EchoActivity)
	require.NoError(t, w.Start())

	// Stop the worker exactly once on every exit path. Draining in-flight tasks
	// flushes all spans through the synchronous exporter before GetSpans; the
	// t.Cleanup guard also covers assertion failures before the explicit stop.
	var stopOnce sync.Once
	stop := func() { stopOnce.Do(w.Stop) }
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	run, err := c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{TaskQueue: otelTestTaskQueue}, EchoWorkflow, "hello")
	require.NoError(t, err)

	var result string
	require.NoError(t, run.Get(ctx, &result))
	require.Equal(t, "HELLO", result, "workflow executed through the otel interceptor must return the activity result")

	// Drain now so the synchronous exporter has flushed all worker-side spans.
	stop()

	spans := exp.GetSpans()
	require.NotEmpty(t, spans, "the Temporal otel interceptor must produce spans")

	names := make([]string, len(spans))
	for i, s := range spans {
		names[i] = s.Name
	}
	found := slices.ContainsFunc(spans, func(s tracetest.SpanStub) bool {
		return strings.Contains(s.Name, "EchoWorkflow") || strings.Contains(s.Name, "EchoActivity")
	})
	require.True(t, found, "expected a workflow/activity span, got: %v", names)
}
