package httpmw

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recovery is a negroni-compatible middleware that catches panics in the
// request goroutine, logs the stack via slog, calls an optional OnPanic
// hook (typically to bump a metrics counter), then writes a 500 response.
//
// It replaces negroni.Recovery so panic events flow through the project's
// slog handler chain (console + OTel + Sentry breadcrumbs/events) and can
// be alerted on via a metrics counter.
type Recovery struct {
	// OnPanic is called with the recovered value before the 500 is
	// written. Typical use is to increment a counter like
	// instrumentation.HTTPPanics.Inc(serviceName).
	OnPanic func(r any)
}

// NewRecovery constructs a Recovery middleware.
func NewRecovery(onPanic func(r any)) *Recovery {
	return &Recovery{OnPanic: onPanic}
}

// ServeHTTP implements negroni.Handler.
func (rec *Recovery) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	defer func() {
		if err := recover(); err != nil {
			stack := debug.Stack()
			slog.ErrorContext(r.Context(), "panic recovered",
				"err", fmt.Sprintf("%v", err),
				"method", r.Method,
				"path", r.URL.Path,
				"stack", string(stack),
			)
			if rec.OnPanic != nil {
				rec.OnPanic(err)
			}
			http.Error(rw, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	}()
	next(rw, r)
}
