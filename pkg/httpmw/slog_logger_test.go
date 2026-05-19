package httpmw

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/urfave/negroni/v3"
)

func TestSlogLoggerLevelByPath(t *testing.T) {
	cases := []struct {
		path     string
		wantLvl  string
		notLevel string
	}{
		{path: "/health/live", wantLvl: "DEBUG", notLevel: "INFO"},
		{path: "/health/ready", wantLvl: "DEBUG", notLevel: "INFO"},
		{path: "/anything-else", wantLvl: "INFO", notLevel: "DEBUG"},
		{path: "/health/", wantLvl: "INFO", notLevel: "DEBUG"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			var buf bytes.Buffer
			prev := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() { slog.SetDefault(prev) })

			n := negroni.New(NewSlogLogger())
			n.UseHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, tc.path, nil).WithContext(context.Background())
			rec := httptest.NewRecorder()
			n.ServeHTTP(rec, req)

			out := buf.String()
			if !strings.Contains(out, "level="+tc.wantLvl) {
				t.Fatalf("want level=%s in log line, got %q", tc.wantLvl, out)
			}
			if strings.Contains(out, "level="+tc.notLevel) {
				t.Fatalf("did not want level=%s in log line, got %q", tc.notLevel, out)
			}
			if !strings.Contains(out, "path="+tc.path) {
				t.Fatalf("want path=%s in log line, got %q", tc.path, out)
			}
			if !strings.Contains(out, "status=200") {
				t.Fatalf("want status=200 in log line, got %q", out)
			}
		})
	}
}
