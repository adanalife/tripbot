package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/feature"
	"github.com/gorilla/mux"
)

// fakeFlags is a FlagClient (+ FlagToggler) seam for the /api/flags handlers.
type fakeFlags struct {
	snap     []feature.Flag
	setCalls []struct {
		key     string
		enabled bool
	}
	setErr error
}

func (f *fakeFlags) Bool(context.Context, string, feature.EvalContext) bool { return false }
func (f *fakeFlags) Snapshot(context.Context) []feature.Flag                { return f.snap }
func (f *fakeFlags) SetEnabled(_ context.Context, key string, enabled bool) error {
	f.setCalls = append(f.setCalls, struct {
		key     string
		enabled bool
	}{key, enabled})
	return f.setErr
}

func flagsRouter(s *Server) *mux.Router {
	r := mux.NewRouter()
	r.Handle("/api/flags", http.HandlerFunc(s.flagsHandler)).Methods("GET")
	r.Handle("/api/flags/{key}", http.HandlerFunc(s.flagToggleHandler)).Methods("POST")
	return r
}

func TestFlagsHandler_Snapshot(t *testing.T) {
	fc := &fakeFlags{snap: []feature.Flag{
		{
			Key:                 "chatbot.report_to_discord",
			Description:         "mirror chat to discord",
			Enabled:             true,
			EnabledForUsernames: []string{"danalol"},
			TargetRemovalDate:   time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		},
		{Key: "ascii_video", Enabled: false},
	}}
	s := New(testConf)
	s.SetFlags(fc)

	rec := httptest.NewRecorder()
	flagsRouter(s).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/flags", nil))

	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	var got struct {
		OK    bool      `json:"ok"`
		Flags []flagDTO `json:"flags"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, rec.Body.String())
	}
	if !got.OK || len(got.Flags) != 2 {
		t.Fatalf("ok=%v flags=%d, want ok=true 2 flags", got.OK, len(got.Flags))
	}
	if got.Flags[0].Key != "chatbot.report_to_discord" || !got.Flags[0].Enabled {
		t.Errorf("flag[0] = %+v", got.Flags[0])
	}
	if got.Flags[0].TargetRemovalDate != "2026-07-01T00:00:00Z" {
		t.Errorf("target removal date = %q", got.Flags[0].TargetRemovalDate)
	}
}

func TestFlagsHandler_NoClientReportsNotOK(t *testing.T) {
	s := New(testConf) // no SetFlags
	rec := httptest.NewRecorder()
	flagsRouter(s).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/flags", nil))

	var got struct {
		OK    bool      `json:"ok"`
		Flags []flagDTO `json:"flags"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.OK || len(got.Flags) != 0 {
		t.Errorf("ok=%v flags=%d, want ok=false empty", got.OK, len(got.Flags))
	}
}

func TestFlagToggleHandler_TogglesViaToggler(t *testing.T) {
	fc := &fakeFlags{}
	s := New(testConf)
	s.SetFlags(fc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/flags/ascii_video",
		strings.NewReader(`{"enabled":true}`))
	flagsRouter(s).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\n%s", rec.Code, rec.Body.String())
	}
	if len(fc.setCalls) != 1 || fc.setCalls[0].key != "ascii_video" || !fc.setCalls[0].enabled {
		t.Errorf("setCalls = %+v, want one ascii_video=true", fc.setCalls)
	}
}

func TestFlagToggleHandler_UnknownKeyIs404(t *testing.T) {
	fc := &fakeFlags{setErr: errors.New("feature flag \"nope\" not found")}
	s := New(testConf)
	s.SetFlags(fc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/flags/nope",
		strings.NewReader(`{"enabled":true}`))
	flagsRouter(s).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestFlagToggleHandler_NonTogglerIs503(t *testing.T) {
	// A client that satisfies FlagClient but not FlagToggler (the in-memory
	// fallback before the Postgres client loads) can't be toggled.
	s := New(testConf)
	s.SetFlags(feature.NewInMemoryClient(nil))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/flags/ascii_video",
		strings.NewReader(`{"enabled":true}`))
	flagsRouter(s).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
