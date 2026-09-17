package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/httpmw"
)

// An unconfigured NATS is not a degraded one — a laptop run with NATS_URL
// unset must not report a dependency down that it was never given.
func TestDepChecksSkipUnconfiguredNats(t *testing.T) {
	for _, tc := range []struct {
		name    string
		natsURL string
		want    []string
	}{
		{"no nats configured", "", []string{"postgres"}},
		{"nats configured", "nats://nats:4222", []string{"postgres", "nats"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New(&c.TripbotConfig{NatsURL: tc.natsURL})
			got := make([]string, 0, len(s.depChecks()))
			for _, chk := range s.depChecks() {
				got = append(got, chk.Name)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("checks = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("checks = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// The point of /health/deps is that a failing dependency is *reported* rather
// than gated on: the body names the dep so the console (and a human with curl)
// can say which one, and the status is 503 without the pod leaving the Service,
// because nothing mounts these checks on /health/ready.
func TestDepsHandlerNamesTheFailedDependency(t *testing.T) {
	h := httpmw.ReadinessHandler(
		httpmw.ReadyCheck{Name: "postgres", Fn: func(context.Context) error { return nil }},
		httpmw.ReadyCheck{Name: "nats", Fn: func(context.Context) error { return errors.New("connection lost") }},
	)
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/health/deps", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
	var body struct {
		OK     bool `json:"ok"`
		Checks []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
			Err  string `json:"error"`
		} `json:"checks"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.OK {
		t.Error("ok = true with a failing dependency")
	}
	if len(body.Checks) != 2 {
		t.Fatalf("checks = %+v, want both reported", body.Checks)
	}
	if !body.Checks[0].OK || body.Checks[0].Name != "postgres" {
		t.Errorf("postgres check = %+v, want a healthy postgres reported by name", body.Checks[0])
	}
	if body.Checks[1].OK || body.Checks[1].Err != "connection lost" {
		t.Errorf("nats check = %+v, want the failure and its reason", body.Checks[1])
	}
}

// natsPing must not claim health from a nil singleton — that is the state a
// pod is in before Connect succeeds, and reporting it healthy is the exact
// dishonesty /health/deps exists to remove.
func TestNatsPingUnconnected(t *testing.T) {
	if err := natsPing(context.Background()); err == nil {
		t.Error("natsPing = nil with no connection, want an error")
	}
}
