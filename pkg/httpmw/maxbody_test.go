package httpmw

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaxBodyRejectsOversizedBody(t *testing.T) {
	const limit = 64
	decode := func(w http.ResponseWriter, r *http.Request) {
		var v map[string]string
		// The handlers in pkg/server answer any decode error with 400.
		if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}

	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"under the cap", `{"k":"v"}`, http.StatusNoContent},
		// Well-formed JSON, so the 400 can only come from the cap.
		{"over the cap", `{"k":"` + strings.Repeat("x", limit) + `"}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			MaxBody(limit).ServeHTTP(rec, req, decode)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
