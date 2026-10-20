package httpmw

import "net/http"

// MaxBody is a negroni-compatible middleware capping every request body at
// its value in bytes. A read past the cap fails with *http.MaxBytesError, so a
// handler's JSON decode errors out instead of buffering whatever a caller sends.
type MaxBody int64

// ServeHTTP implements negroni.Handler.
func (n MaxBody) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	r.Body = http.MaxBytesReader(rw, r.Body, int64(n))
	next(rw, r)
}
