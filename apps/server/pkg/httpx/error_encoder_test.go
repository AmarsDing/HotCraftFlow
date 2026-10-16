package httpx

import (
	"database/sql"
	stderrors "errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-kratos/kratos/v2/errors"
)

func TestNormalize_PassthroughKratosError(t *testing.T) {
	se := errors.Forbidden("FORBIDDEN", "no permission")
	got := normalize(se)
	var target *errors.Error
	if !stderrors.As(got, &target) || target.Reason != "FORBIDDEN" {
		t.Fatalf("expected passthrough FORBIDDEN, got %v", got)
	}
}

func TestNormalize_NotFound(t *testing.T) {
	cases := []error{
		sql.ErrNoRows,
		fmt.Errorf("query device: %w", sql.ErrNoRows),
		stderrors.New("ent: device not found"),
		stderrors.New("no rows in result set"),
	}
	for _, err := range cases {
		got := normalize(err)
		var se *errors.Error
		if !stderrors.As(got, &se) {
			t.Fatalf("expected kratos error for %v", err)
		}
		if se.Code != 404 {
			t.Fatalf("expected 404 for %v, got %d", err, se.Code)
		}
	}
}

func TestNormalize_BadRequest(t *testing.T) {
	cases := []error{
		stderrors.New("ent: validator failed for field \"name\""),
		stderrors.New("name is required"),
		stderrors.New("invalid argument"),
		stderrors.New("alarm cannot be marked"),
	}
	for _, err := range cases {
		got := normalize(err)
		var se *errors.Error
		if !stderrors.As(got, &se) {
			t.Fatalf("expected kratos error for %v", err)
		}
		if se.Code != 400 {
			t.Fatalf("expected 400 for %v, got %d", err, se.Code)
		}
	}
}

func TestNormalize_UnknownKeeps500(t *testing.T) {
	got := normalize(stderrors.New("some internal failure"))
	var se *errors.Error
	if stderrors.As(got, &se) && se.Reason != "" {
		t.Fatalf("expected unknown error passthrough, got reason %q", se.Reason)
	}
}

func TestErrorEncoder_WritesMappedStatus(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/x", nil)
	ErrorEncoder(w, r, stderrors.New("ent: device not found"))
	if w.Code != 404 {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "NOT_FOUND") {
		t.Fatalf("expected NOT_FOUND in body, got %s", w.Body.String())
	}
}
