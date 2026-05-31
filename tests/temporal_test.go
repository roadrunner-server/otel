package tests

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/roadrunner-server/otel/v6"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/client"
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

// TestPlugin_TemporalInterceptor_DevServer runs a real Temporal workflow through
// the otel plugin's WorkerInterceptor against a `temporal server start-dev`
// instance, and asserts that the plugin exported the spans the interceptor
// produced. It is skipped unless TEMPORAL_ADDRESS points at a running dev
// server (the CI workflow sets it to 127.0.0.1:7233).
func TestPlugin_TemporalInterceptor_DevServer(t *testing.T) {
	addr := os.Getenv("TEMPORAL_ADDRESS")
	if addr == "" {
		t.Skip("TEMPORAL_ADDRESS not set; run `temporal server start-dev` and set TEMPORAL_ADDRESS=127.0.0.1:7233 to run this test")
	}

	c, err := client.Dial(client.Options{HostPort: addr, Namespace: "default"})
	require.NoError(t, err, "dial temporal dev server at %s", addr)
	defer c.Close()

	// Redirect stdout so the plugin's stdout span exporter writes into a pipe we
	// can inspect. testing.T buffers its own output, so test results are still
	// reported correctly. The drain goroutine prevents the pipe from blocking.
	origStdout := os.Stdout
	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	defer func() { os.Stdout = origStdout }()
	os.Stdout = pw

	var captured bytes.Buffer
	drained := make(chan struct{})
	go func() {
		_, _ = io.Copy(&captured, pr)
		close(drained)
	}()

	p := &otel.Plugin{}
	require.NoError(t, p.Init(newConfigurer(&otel.Config{Exporter: otel.Exporter("stdout")}), mockLogger{}))

	w := worker.New(c, otelTestTaskQueue, worker.Options{
		Interceptors: []interceptor.WorkerInterceptor{p.WorkerInterceptor()},
	})
	w.RegisterWorkflow(EchoWorkflow)
	w.RegisterActivity(EchoActivity)
	require.NoError(t, w.Start())
	defer w.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	run, err := c.ExecuteWorkflow(ctx, client.StartWorkflowOptions{TaskQueue: otelTestTaskQueue}, EchoWorkflow, "hello")
	require.NoError(t, err)

	var result string
	require.NoError(t, run.Get(ctx, &result))
	require.Equal(t, "HELLO", result, "workflow executed through the otel interceptor must return the activity result")

	// Flush batched spans through the plugin's exporter, then read what landed.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer stopCancel()
	require.NoError(t, p.Stop(stopCtx))

	_ = pw.Close()
	<-drained
	os.Stdout = origStdout

	out := captured.String()
	require.NotEmpty(t, out, "the otel plugin must export spans produced by the Temporal interceptor")
	require.Contains(t, out, "EchoWorkflow", "exported spans must reference the executed workflow")
}
