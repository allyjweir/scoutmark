package tracing

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "scoutmark"

// Tracer returns the application-wide tracer.
func Tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

// Init sets up the OpenTelemetry trace pipeline.
// When HONEYCOMB_API_KEY is set, traces are exported to Honeycomb via gRPC (TLS).
// Otherwise, traces are exported to a local Jaeger instance via OTLP gRPC (plaintext).
// Returns a shutdown function that should be called on application exit.
func Init(ctx context.Context, serviceName, version string) (func(context.Context) error, error) {
	apiKey := os.Getenv("HONEYCOMB_API_KEY")

	var exporter sdktrace.SpanExporter
	var err error

	if apiKey != "" {
		// Honeycomb exporter (TLS)
		dataset := os.Getenv("HONEYCOMB_DATASET")
		if dataset == "" {
			dataset = "scoutmark"
		}

		exporter, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint("api.honeycomb.io:443"),
			otlptracegrpc.WithHeaders(map[string]string{
				"x-honeycomb-team":    apiKey,
				"x-honeycomb-dataset": dataset,
			}),
		)
		if err != nil {
			return nil, fmt.Errorf("creating honeycomb exporter: %w", err)
		}
		fmt.Fprintf(os.Stderr, "tracing: exporting to Honeycomb\n")
	} else {
		// Local Jaeger via OTLP gRPC (plaintext)
		jaegerEndpoint := envOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")

		exporter, err = otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(jaegerEndpoint),
			otlptracegrpc.WithInsecure(),
		)
		if err != nil {
			return nil, fmt.Errorf("creating jaeger exporter: %w", err)
		}
		fmt.Fprintf(os.Stderr, "tracing: exporting to Jaeger at %s (UI at http://localhost:16686)\n", jaegerEndpoint)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(version),
			attribute.String("environment", envOrDefault("ENVIRONMENT", "development")),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		// Sample everything in dev; configure down in production
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

// HTTPMiddleware wraps an http.Handler with tracing.
// Creates a span per request with rich attributes — the Charity Majors way.
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := Tracer().Start(r.Context(), fmt.Sprintf("%s %s", r.Method, r.URL.Path),
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.url", r.URL.String()),
				attribute.String("http.path", r.URL.Path),
				attribute.String("http.user_agent", r.UserAgent()),
				attribute.String("http.remote_addr", r.RemoteAddr),
				attribute.String("http.host", r.Host),
			),
		)
		defer span.End()

		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r.WithContext(ctx))

		span.SetAttributes(
			attribute.Int("http.status_code", sw.status),
			attribute.Int("http.response_size", sw.written),
		)
	})
}

// SpanFromContext returns the current span and adds user context attributes.
func AddUserAttrs(ctx context.Context, userID, displayName string) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(
		attribute.String("user.id", userID),
		attribute.String("user.display_name", displayName),
	)
}

// AddSessionAttrs adds session-related attributes to the current span.
func AddSessionAttrs(ctx context.Context, sessionID, patrolID string) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(
		attribute.String("session.id", sessionID),
		attribute.String("patrol.id", patrolID),
	)
}

// RecordError records an error on the current span.
func RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
}

type statusWriter struct {
	http.ResponseWriter
	status  int
	written int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.written += n
	return n, err
}

// Hijack implements http.Hijacker, required for WebSocket upgrades.
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
