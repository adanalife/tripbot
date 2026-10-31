package httpmw

import (
	sentrynegroni "github.com/getsentry/sentry-go/negroni"
	"github.com/urfave/negroni/v3"
)

// NewSentry returns the Sentry negroni middleware that reports a handler
// panic and then re-panics it. Sentry sits inside Recovery in the chain, so
// re-panicking is what lets Recovery see the panic, bump the panic counter,
// and answer 500; without it Sentry swallows the panic and the client gets
// an empty 200.
func NewSentry() negroni.Handler {
	return sentrynegroni.New(sentrynegroni.Options{Repanic: true})
}
