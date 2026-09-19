package errors

import (
	"strings"

	"github.com/getsentry/sentry-go"
)

// noiseTagKey is the Sentry tag carrying an event's environment class. A tagged
// event is one whose text describes the cluster rather than a defect in this
// code — a neighbour pod restarting, a platform refusing the call, the database
// cycling. Tagged rather than dropped: the first event of a real outage looks
// exactly like the hundredth event of routine churn, so silencing the pattern
// would silence the outage too. `!noise:*` in Sentry search is the defect-only
// view; `noise:peer-unavailable` is the churn.
const noiseTagKey = "noise"

// Environment classes. One per recovery story, not one per error string —
// splitting finer would just rebuild the issue list inside the tag.
const (
	noisePeerUnavailable = "peer-unavailable" // nothing listening: refused, reset, EOF
	noiseTimeout         = "timeout"          // accepted and never answered
	noiseDNS             = "dns"              // the name did not resolve
	noiseUpstream        = "upstream"         // a neighbour answered, its own upstream had not
)

// noisePatterns maps a substring of an error's text to its class. Matched
// against the captured exception values and the slog message, lowercased.
// Ordered most-specific first: "connection reset by peer" must not be read as a
// timeout by a later, looser pattern.
var noisePatterns = []struct {
	substr, class string
}{
	{"connect: connection refused", noisePeerUnavailable},
	{"connection reset by peer", noisePeerUnavailable},
	{"unexpected eof", noisePeerUnavailable},
	{"broken pipe", noisePeerUnavailable},
	{"no route to host", noisePeerUnavailable},
	{"context deadline exceeded", noiseTimeout},
	{"client.timeout exceeded", noiseTimeout},
	{"i/o timeout", noiseTimeout},
	{"no such host", noiseDNS},
	{"failure in name resolution", noiseDNS},
	{"server misbehaving", noiseDNS},
	{"unexpected status 502", noiseUpstream},
	{"unexpected status 503", noiseUpstream},
	{"unexpected status 504", noiseUpstream},
}

// classifyNoise returns the environment class of an event, or "" when nothing
// about it says the cluster rather than the code.
func classifyNoise(event *sentry.Event) string {
	if event == nil {
		return ""
	}
	texts := make([]string, 0, len(event.Exception)+1)
	texts = append(texts, event.Message)
	for _, e := range event.Exception {
		texts = append(texts, e.Value)
	}
	for _, text := range texts {
		lower := strings.ToLower(text)
		for _, p := range noisePatterns {
			if strings.Contains(lower, p.substr) {
				return p.class
			}
		}
	}
	return ""
}

// tagNoise stamps the class onto the event when it has one. An event that
// already carries the tag keeps it — a caller that classified its own failure
// knows more than a substring match does.
func tagNoise(event *sentry.Event) {
	if event == nil || event.Tags[noiseTagKey] != "" {
		return
	}
	class := classifyNoise(event)
	if class == "" {
		return
	}
	if event.Tags == nil {
		event.Tags = map[string]string{}
	}
	event.Tags[noiseTagKey] = class
}
