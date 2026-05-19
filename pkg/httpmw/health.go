package httpmw

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// readyCheckTimeout caps how long the whole /health/ready handler can
// spend running its checks. Kubelet's default readiness timeout is 1s;
// 2s here leaves enough headroom for one slow check to complete before
// the probe gets cut, without letting a wedged dep hang the handler.
const readyCheckTimeout = 2 * time.Second

// ReadyCheck is one readiness condition the server should report on.
// Name is rendered into the JSON response so probes (and humans) can
// see which dep failed; Fn returns nil for healthy, error otherwise.
type ReadyCheck struct {
	Name string
	Fn   func(ctx context.Context) error
}

// LivenessHandler returns 200 OK unconditionally. /health/live is the
// kubelet's "is the process up at all?" signal — a failure here means
// the pod gets restarted, so this should only fail if the process is
// genuinely unrecoverable. Today that bar is "the goroutine is running
// the handler at all," which 200 satisfies.
func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK\n"))
	}
}

// ReadinessHandler returns a handler that runs each check and reports
// 200 if all pass, 503 if any fail. The JSON body lists per-check
// status so a failing probe is debuggable from `kubectl describe pod`'s
// probe output. With no checks, the handler always reports 200 —
// equivalent to LivenessHandler but distinguishable on the URL.
func ReadinessHandler(checks ...ReadyCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readyCheckTimeout)
		defer cancel()

		type result struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
			Err  string `json:"error,omitempty"`
		}
		out := make([]result, 0, len(checks))
		allOK := true
		for _, c := range checks {
			err := c.Fn(ctx)
			res := result{Name: c.Name, OK: err == nil}
			if err != nil {
				res.Err = err.Error()
				allOK = false
			}
			out = append(out, res)
		}

		status := http.StatusOK
		if !allOK {
			status = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(struct {
			OK     bool     `json:"ok"`
			Checks []result `json:"checks"`
		}{allOK, out})
	}
}
