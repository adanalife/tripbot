package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// onscreensVersion is the whole of what an onscreens deploy row carries, so a
// read that can't name a version must fail rather than record "".
func TestOnscreensVersion(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		want    string
		wantErr string
	}{
		{
			name:   "the build tag",
			status: http.StatusOK,
			body:   `{"tag":"5.26.0","sha":"abc","built_at":"","started_at":"2026-10-06T00:00:00Z"}`,
			want:   "5.26.0",
		},
		{name: "a non-200", status: http.StatusServiceUnavailable, body: "", wantErr: "503"},
		{name: "an empty tag", status: http.StatusOK, body: `{"tag":""}`, wantErr: "empty tag"},
		{name: "not JSON", status: http.StatusOK, body: "ok", wantErr: "/version"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/version" {
					t.Errorf("path = %q, want /version", r.URL.Path)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			got, err := onscreensVersion(context.Background(), strings.TrimPrefix(srv.URL, "http://"))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want one naming %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("onscreensVersion = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}
