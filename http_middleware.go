package otel

import (
	"context"
	"net/http"

	rrcontext "github.com/roadrunner-server/context"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// type alias for the middleware
type httpMiddleware func(http.Handler) http.Handler

func HTTPHandler(next http.Handler, middleware httpMiddleware) http.Handler {
	return middleware(next)
}

func httpWrapper(prop propagation.TextMapPropagator, tr trace.TracerProvider, sn string) httpMiddleware {
	return func(h http.Handler) http.Handler {
		// init otelhttp handler only once
		handler := otelhttp.NewHandler(h, "",
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.RequestURI
			}),
			otelhttp.WithSpanOptions(
				trace.WithSpanKind(trace.SpanKindServer),
			),
			otelhttp.WithPropagators(prop),
			otelhttp.WithTracerProvider(tr),
			otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents))

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), rrcontext.OtelTracerNameKey, sn)
			// have effect only if the span started outside
			// if the OTEL middleware is the first in the line, has no effect (we haven't yet started a span)
			prop.Inject(ctx, propagation.HeaderCarrier(r.Header))
			handler.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
