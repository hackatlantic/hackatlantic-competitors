package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func testTelemetry(t *testing.T) *Telemetry {
	t.Helper()
	telemetry := &Telemetry{meter: otel.Meter(serviceName)}
	if err := telemetry.createInstruments(); err != nil {
		t.Fatalf("create instruments: %v", err)
	}
	return telemetry
}

func TestHTTPMetricsKeepRouteTemplates(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	telemetry := &Telemetry{meter: provider.Meter(serviceName)}
	if err := telemetry.createInstruments(); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/applications/{applicationId}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/applications/private-id?email=private@example.test", nil)
	telemetry.HTTPMiddleware(slog.New(slog.NewJSONHandler(&logs, nil)), mux).ServeHTTP(httptest.NewRecorder(), request)
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	for _, scope := range data.ScopeMetrics {
		for _, instrument := range scope.Metrics {
			if instrument.Name != "http.server.requests" {
				continue
			}
			points := instrument.Data.(metricdata.Sum[int64]).DataPoints
			if len(points) != 1 {
				t.Fatalf("request points = %d", len(points))
			}
			route, _ := points[0].Attributes.Value("http.route")
			if route.AsString() != "GET /v1/applications/{applicationId}" {
				t.Fatalf("route = %q", route.AsString())
			}
			if strings.Contains(logs.String(), "private") {
				t.Fatal("request data leaked to logs")
			}
			return
		}
	}
	t.Fatal("request metric missing")
}

func TestHTTPMiddlewarePreservesSafeRequestID(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := testTelemetry(t).HTTPMiddleware(logger, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if got := RequestID(request.Context()); got != "release_123" {
			t.Fatalf("context request ID = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("X-Request-ID", "release_123")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if got := response.Header().Get("X-Request-ID"); got != "release_123" {
		t.Fatalf("response request ID = %q", got)
	}
	if !strings.Contains(logs.String(), `"request_id":"release_123"`) {
		t.Fatalf("request ID missing from structured log: %s", logs.String())
	}
}

func TestTracePrivacyAndLogCorrelation(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		_ = provider.Shutdown(context.Background())
	})
	var logs bytes.Buffer
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/applications/{applicationId}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("GET /v1/claim/{token}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := testTelemetry(t).HTTPMiddleware(slog.New(slog.NewJSONHandler(&logs, nil)), mux)
	request := httptest.NewRequest(http.MethodGet, "/v1/applications/private-id?email=private@example.test", nil)
	request.Header.Set("Authorization", "Bearer private-jwt")
	request.Header.Set("User-Agent", "private-agent")
	request.Header.Set("Baggage", "email=private@example.test")
	request.Header.Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	handler.ServeHTTP(httptest.NewRecorder(), request)
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Name != "GET /v1/applications/{applicationId}" {
		t.Fatalf("expected one route-template span, got %v", spans)
	}
	if spans[0].SpanContext.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatal("incoming trace context was not preserved")
	}
	if !strings.Contains(logs.String(), spans[0].SpanContext.TraceID().String()) {
		t.Fatal("trace ID is missing from request log")
	}
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/claim/private-token", nil))
	if len(exporter.GetSpans()) != 1 {
		t.Fatal("claim routes must not create traces")
	}
	encoded, err := json.Marshal(exporter.GetSpans())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded)+logs.String(), "private") {
		t.Fatal("PII or bearer material leaked into spans or logs")
	}
}

func TestTelemetryHeartbeatWithoutTraffic(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	telemetry := &Telemetry{meter: provider.Meter(serviceName)}
	if err := telemetry.createInstruments(); err != nil {
		t.Fatal(err)
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	for _, scope := range data.ScopeMetrics {
		for _, instrument := range scope.Metrics {
			if instrument.Name == "hackatlantic.telemetry.up" {
				points := instrument.Data.(metricdata.Gauge[int64]).DataPoints
				if len(points) != 1 || points[0].Value != 1 {
					t.Fatal("heartbeat must emit one independently of HTTP traffic")
				}
				return
			}
		}
	}
	t.Fatal("heartbeat missing")
}

func TestStatusWriterKeepsFirstStatus(t *testing.T) {
	response := httptest.NewRecorder()
	writer := &statusWriter{ResponseWriter: response}
	writer.WriteHeader(http.StatusCreated)
	writer.WriteHeader(http.StatusInternalServerError)
	if writer.status != http.StatusCreated || response.Code != http.StatusCreated {
		t.Fatal("metrics must record the status actually sent to the client")
	}
	if writer.Unwrap() != response {
		t.Fatal("response controller cannot reach underlying writer")
	}
}

func TestHTTPMiddlewareReplacesUnsafeRequestID(t *testing.T) {
	handler := testTelemetry(t).HTTPMiddleware(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("X-Request-ID", "contains applicant@example.test")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	got := response.Header().Get("X-Request-ID")
	if got == "" || got == request.Header.Get("X-Request-ID") {
		t.Fatalf("unsafe request ID was not replaced: %q", got)
	}
}

func TestLifecycleEventUsesRouteTemplates(t *testing.T) {
	tests := map[string]struct {
		method string
		route  string
		status int
		want   string
	}{
		"submission": {http.MethodPost, "POST /v1/applications/{applicationId}/submit", http.StatusOK, "application.submitted"},
		"decision":   {http.MethodPost, "POST /v1/admin/decisions/{decisionId}/release", http.StatusOK, "decision.released"},
		"pass":       {http.MethodPost, "POST /v1/admin/attendees/{attendeeId}/passes", http.StatusCreated, "pass.issued"},
		"redemption": {http.MethodPost, "POST /v1/redemptions", http.StatusOK, "redemption.completed"},
		"failure":    {http.MethodPost, "POST /v1/redemptions", http.StatusConflict, ""},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := lifecycleEvent(test.method, test.route, test.status); got != test.want {
				t.Fatalf("lifecycle event = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNoEndpointLeavesTelemetryLocal(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
	telemetry, err := New(context.Background(), Config{Version: "test", Environment: "test"})
	if err != nil {
		t.Fatalf("new telemetry: %v", err)
	}
	if telemetry.tracerProvider != nil || telemetry.meterProvider != nil || telemetry.loggerProvider != nil {
		t.Fatal("local telemetry unexpectedly created exporters")
	}
	base := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	if telemetry.Logger(base) != base {
		t.Fatal("local logger should not be wrapped without an OTLP endpoint")
	}
}
