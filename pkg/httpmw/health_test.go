package httpmw

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLivenessHandler(t *testing.T) {
	rec := httptest.NewRecorder()
	LivenessHandler()(rec, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestReadinessHandlerAllPass(t *testing.T) {
	h := ReadinessHandler(
		ReadyCheck{Name: "a", Fn: func(context.Context) error { return nil }},
		ReadyCheck{Name: "b", Fn: func(context.Context) error { return nil }},
	)
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		OK     bool `json:"ok"`
		Checks []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"checks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !body.OK || len(body.Checks) != 2 {
		t.Fatalf("body = %+v, want ok=true and 2 checks", body)
	}
}

func TestReadinessHandlerCheckFails(t *testing.T) {
	h := ReadinessHandler(
		ReadyCheck{Name: "a", Fn: func(context.Context) error { return nil }},
		ReadyCheck{Name: "b", Fn: func(context.Context) error { return errors.New("nope") }},
	)
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if !contains(rec.Body.String(), `"name":"b"`) || !contains(rec.Body.String(), `"error":"nope"`) {
		t.Fatalf("body should name the failing check, got %s", rec.Body.String())
	}
}

func TestReadinessHandlerNoChecks(t *testing.T) {
	rec := httptest.NewRecorder()
	ReadinessHandler()(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for empty check list", rec.Code)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
