package observability

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
)

func testTelemetry(t *testing.T) *Telemetry {
	t.Helper()
	telemetry := &Telemetry{meter: otel.Meter(serviceName)}
	if err := telemetry.createInstruments(); err != nil {
		t.Fatalf("create instruments: %v", err)
	}
	return telemetry
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
