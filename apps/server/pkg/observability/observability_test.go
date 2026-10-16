package observability

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	kratoserrors "github.com/go-kratos/kratos/v2/errors"
)

func TestReadyHandler_AllChecksPass(t *testing.T) {
	h := ReadyHandler(map[string]CheckFunc{
		"postgres": func(ctx context.Context) error { return nil },
		"redis":    func(ctx context.Context) error { return nil },
	}, time.Second)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "ready" {
		t.Fatalf("expected status ready, got %q", body.Status)
	}
	for name, v := range body.Checks {
		if v != "ok" {
			t.Fatalf("check %s expected ok, got %q", name, v)
		}
	}
}

func TestReadyHandler_OneCheckFails(t *testing.T) {
	h := ReadyHandler(map[string]CheckFunc{
		"postgres": func(ctx context.Context) error { return nil },
		"influxdb": func(ctx context.Context) error { return errors.New("connection refused") },
	}, time.Second)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	var body struct {
		Status string            `json:"status"`
		Checks map[string]string `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "not_ready" {
		t.Fatalf("expected status not_ready, got %q", body.Status)
	}
	if body.Checks["influxdb"] != "connection refused" {
		t.Fatalf("expected influxdb error detail, got %q", body.Checks["influxdb"])
	}
	if body.Checks["postgres"] != "ok" {
		t.Fatalf("expected postgres ok, got %q", body.Checks["postgres"])
	}
}

func TestReadyHandler_NilCheckSkipped(t *testing.T) {
	h := ReadyHandler(map[string]CheckFunc{
		"redis": nil,
	}, time.Second)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestReadyHandler_CheckTimeout(t *testing.T) {
	h := ReadyHandler(map[string]CheckFunc{
		"slow": func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
				return nil
			}
		},
	}, 50*time.Millisecond)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestHTTPStatusFromError(t *testing.T) {
	if got := httpStatusFromError(nil); got != http.StatusOK {
		t.Fatalf("nil error: expected 200, got %d", got)
	}
	if got := httpStatusFromError(kratoserrors.BadRequest("TEST", "bad")); got != http.StatusBadRequest {
		t.Fatalf("kratos 400 error: expected 400, got %d", got)
	}
	if got := httpStatusFromError(kratoserrors.Unauthorized("TEST", "unauth")); got != http.StatusUnauthorized {
		t.Fatalf("kratos 401 error: expected 401, got %d", got)
	}
	if got := httpStatusFromError(errors.New("boom")); got != http.StatusInternalServerError {
		t.Fatalf("plain error: expected 500, got %d", got)
	}
}
