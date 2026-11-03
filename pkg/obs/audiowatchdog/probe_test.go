package audiowatchdog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The probe is what decides the stream can leave the fallback bed, so each way
// a station can look alive without serving audio must read unreachable — and
// the one way that is alive must not. An Icecast edge that refuses Go's default
// User-Agent closes the connection before any response, which is the case that
// once stranded the stream on the fallback.
func TestDefaultRadioReachable(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		want    bool
	}{
		{"serving bytes", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ID3"))
		}, true},
		{"partial content", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("x"))
		}, true},
		{"refuses Go's default user agent", func(w http.ResponseWriter, r *http.Request) {
			if r.UserAgent() != radioProbeUserAgent {
				panic(http.ErrAbortHandler) // drop the connection, as the edge does
			}
			_, _ = w.Write([]byte("x"))
		}, true},
		{"error status", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("down"))
		}, false},
		{"200 with no body", func(http.ResponseWriter, *http.Request) {}, false},
		{"handshake then silence", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			if got := defaultRadioReachable(ctx, srv.URL); got != tc.want {
				t.Errorf("reachable = %v, want %v", got, tc.want)
			}
		})
	}
}
