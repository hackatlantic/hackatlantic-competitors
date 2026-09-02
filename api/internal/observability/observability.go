// Package observability configures privacy-safe OpenTelemetry telemetry.
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

const serviceName = "hackatlantic-ats-api"

// Config identifies a deployed build without exposing runtime secrets.
type Config struct {
	Version     string
	GitSHA      string
	Environment string
}

// Telemetry owns providers and instruments that must be shut down cleanly.
type Telemetry struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	loggerProvider *sdklog.LoggerProvider
	meter          metric.Meter
	requests       metric.Int64Counter
	duration       metric.Float64Histogram
	active         metric.Int64UpDownCounter
	businessEvents metric.Int64Counter
}

// New configures OTLP exporters using the standard OTEL_EXPORTER_OTLP_*
// environment variables. With no endpoint, the global no-op providers remain
// active so local development requires no collector.
func New(ctx context.Context, config Config) (*Telemetry, error) {
	telemetry := &Telemetry{meter: otel.Meter(serviceName)}
	if strings.TrimSpace(config.Environment) == "" {
		config.Environment = "development"
	}
	if strings.TrimSpace(config.Version) == "" {
		config.Version = "dev"
	}

	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(config.Version),
		attribute.String("deployment.environment.name", config.Environment),
		// Each process owns its cumulative counters, including during rolling deployments.
		attribute.String("service.instance.id", newRequestID()),
		attribute.String("vcs.ref.head.revision", strings.TrimSpace(config.GitSHA)),
	))
	if err != nil {
		return nil, fmt.Errorf("build telemetry resource: %w", err)
	}

	if signalEndpointConfigured("TRACES") {
		traceExporter, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("configure OTLP trace exporter: %w", err)
		}
		telemetry.tracerProvider = sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(traceExporter),
			sdktrace.WithResource(res),
		)
		otel.SetTracerProvider(telemetry.tracerProvider)
	}

	if signalEndpointConfigured("METRICS") {
		metricExporter, err := otlpmetrichttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("configure OTLP metric exporter: %w", err)
		}
		telemetry.meterProvider = sdkmetric.NewMeterProvider(
			sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(30*time.Second))),
			sdkmetric.WithResource(res),
		)
		otel.SetMeterProvider(telemetry.meterProvider)
		telemetry.meter = telemetry.meterProvider.Meter(serviceName)
	}

	if signalEndpointConfigured("LOGS") {
		logExporter, err := otlploghttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("configure OTLP log exporter: %w", err)
		}
		telemetry.loggerProvider = sdklog.NewLoggerProvider(
			sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
			sdklog.WithResource(res),
		)
	}

	if err := telemetry.createInstruments(); err != nil {
		return nil, err
	}
	return telemetry, nil
}

func signalEndpointConfigured(signal string) bool {
	return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != "" ||
		strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_"+signal+"_ENDPOINT")) != ""
}

// Logger keeps JSON logs on stdout for the platform log stream and mirrors
// the same privacy-reviewed records to OTLP when the logs exporter is enabled.
func (t *Telemetry) Logger(base *slog.Logger) *slog.Logger {
	if t.loggerProvider == nil {
		return base
	}
	return slog.New(multiHandler{
		base.Handler(),
		otelslog.NewHandler(serviceName, otelslog.WithLoggerProvider(t.loggerProvider)),
	})
}

func (t *Telemetry) createInstruments() error {
	var err error
	t.requests, err = t.meter.Int64Counter("http.server.requests", metric.WithUnit("{request}"))
	if err != nil {
		return fmt.Errorf("create request counter: %w", err)
	}
	t.duration, err = t.meter.Float64Histogram("http.server.duration", metric.WithUnit("ms"),
		metric.WithExplicitBucketBoundaries(25, 50, 100, 250, 500, 750, 1000, 1500, 2500, 5000, 10000))
	if err != nil {
		return fmt.Errorf("create request duration histogram: %w", err)
	}
	t.active, err = t.meter.Int64UpDownCounter("http.server.active_requests", metric.WithUnit("{request}"))
	if err != nil {
		return fmt.Errorf("create active request counter: %w", err)
	}
	t.businessEvents, err = t.meter.Int64Counter("ats.lifecycle.events", metric.WithUnit("{event}"))
	if err != nil {
		return fmt.Errorf("create lifecycle event counter: %w", err)
	}
	up, err := t.meter.Int64ObservableGauge("hackatlantic.telemetry.up")
	if err != nil {
		return err
	}
	_, err = t.meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		observer.ObserveInt64(up, 1)
		return nil
	}, up)
	if err != nil {
		return err
	}
	return nil
}

// Shutdown flushes metrics and spans before process termination.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	var errs []error
	if t.loggerProvider != nil {
		errs = append(errs, t.loggerProvider.Shutdown(ctx))
	}
	if t.meterProvider != nil {
		errs = append(errs, t.meterProvider.Shutdown(ctx))
	}
	if t.tracerProvider != nil {
		errs = append(errs, t.tracerProvider.Shutdown(ctx))
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

type multiHandler []slog.Handler

func (handlers multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (handlers multiHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, handler := range handlers {
		if handler.Enabled(ctx, record.Level) {
			if err := handler.Handle(ctx, record.Clone()); err != nil {
				return err
			}
		}
	}
	return nil
}

func (handlers multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	derived := make(multiHandler, 0, len(handlers))
	for _, handler := range handlers {
		derived = append(derived, handler.WithAttrs(attrs))
	}
	return derived
}

func (handlers multiHandler) WithGroup(name string) slog.Handler {
	derived := make(multiHandler, 0, len(handlers))
	for _, handler := range handlers {
		derived = append(derived, handler.WithGroup(name))
	}
	return derived
}

// RegisterPoolMetrics exposes connection counts only. Query text and bound
// values are intentionally never included.
func (t *Telemetry) RegisterPoolMetrics(pool *pgxpool.Pool) error {
	open, err := t.meter.Int64ObservableGauge("db.client.connections.open", metric.WithUnit("{connection}"))
	if err != nil {
		return err
	}
	inUse, err := t.meter.Int64ObservableGauge("db.client.connections.in_use", metric.WithUnit("{connection}"))
	if err != nil {
		return err
	}
	maximum, err := t.meter.Int64ObservableGauge("db.client.connections.max", metric.WithUnit("{connection}"))
	if err != nil {
		return err
	}
	_, err = t.meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		stats := pool.Stat()
		observer.ObserveInt64(open, int64(stats.TotalConns()))
		observer.ObserveInt64(inUse, int64(stats.AcquiredConns()))
		observer.ObserveInt64(maximum, int64(stats.MaxConns()))
		return nil
	}, open, inUse, maximum)
	return err
}

type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	if status >= 100 && status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Preserve access to optional HTTP features through http.ResponseController.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	written, err := w.ResponseWriter.Write(data)
	w.bytes += written
	return written, err
}

// HTTPMiddleware provides low-cardinality RED metrics and structured logs.
// Only explicitly selected attributes are exported: never URLs, query strings,
// headers, bodies, IPs, or User-Agent. Do not wrap the mux in an automatic HTTP
// instrumenter: it clones requests (losing the matched Pattern here) and may
// capture raw URLs containing applicant or claim data.
func (t *Telemetry) HTTPMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		started := time.Now()
		requestID := safeRequestID(request.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = newRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		request = request.WithContext(context.WithValue(request.Context(), requestIDKey{}, requestID))
		method := safeMethod(request.Method)
		var span trace.Span
		// TraceContext deliberately excludes baggage, which can contain user data.
		if !strings.HasPrefix(request.URL.Path, "/v1/claim/") {
			ctx := propagation.TraceContext{}.Extract(request.Context(), propagation.HeaderCarrier(request.Header))
			ctx, span = otel.Tracer(serviceName).Start(ctx, method,
				trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attribute.String("http.request.method", method)))
			request = request.WithContext(ctx)
		}

		writer := &statusWriter{ResponseWriter: w}
		t.active.Add(request.Context(), 1)
		defer func() {
			t.active.Add(request.Context(), -1)
			if span != nil {
				span.End()
			}
		}()
		next.ServeHTTP(writer, request)
		if writer.status == 0 {
			writer.status = http.StatusOK
		}

		route := request.Pattern
		if route == "" {
			route = "unmatched"
		}
		attrs := metric.WithAttributes(
			attribute.String("http.request.method", method),
			attribute.String("http.route", route),
			attribute.String("http.response.status_class", strconv.Itoa(writer.status/100)+"xx"),
		)
		t.requests.Add(request.Context(), 1, attrs)
		t.duration.Record(request.Context(), float64(time.Since(started).Microseconds())/1000, attrs)
		if event := lifecycleEvent(request.Method, route, writer.status); event != "" {
			t.businessEvents.Add(request.Context(), 1, metric.WithAttributes(attribute.String("event.name", event)))
		}

		if span != nil {
			span.SetName(route)
			span.SetAttributes(attribute.String("http.route", route), attribute.Int("http.response.status_code", writer.status))
			if writer.status >= 500 {
				span.SetStatus(codes.Error, "server error")
			}
		}
		spanContext := trace.SpanFromContext(request.Context()).SpanContext()
		logger.InfoContext(request.Context(), "http request",
			"request_id", requestID,
			"trace_id", spanContext.TraceID().String(),
			"method", method,
			"route", route,
			"status", writer.status,
			"response_bytes", writer.bytes,
			"duration_ms", float64(time.Since(started).Microseconds())/1000,
		)
	})
}

func safeMethod(method string) string {
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT", "TRACE":
		return method
	default:
		return "OTHER"
	}
}

func lifecycleEvent(method, route string, status int) string {
	if status < 200 || status >= 300 {
		return ""
	}
	switch method + " " + route {
	case "POST POST /v1/applications/{applicationId}/submit":
		return "application.submitted"
	case "POST POST /v1/admin/decisions/{decisionId}/release":
		return "decision.released"
	case "POST POST /v1/admin/attendees/{attendeeId}/passes":
		return "pass.issued"
	case "POST POST /v1/redemptions":
		return "redemption.completed"
	default:
		return ""
	}
}

type requestIDKey struct{}

// RequestID returns the correlation ID assigned by HTTPMiddleware.
func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

func safeRequestID(candidate string) string {
	if candidate == "" || len(candidate) > 128 {
		return ""
	}
	for _, char := range candidate {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '-' && char != '_' {
			return ""
		}
	}
	return candidate
}

func newRequestID() string {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(random)
}
