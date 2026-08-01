package errors

import (
	"testing"

	"github.com/getsentry/sentry-go"
)

// fakeConfig stands in for TripbotConfig / OnscreensServerConfig — the whole
// config.Config interface is these two predicates.
type fakeConfig struct {
	prod, staging bool
}

func (f fakeConfig) IsProduction() bool { return f.prod }
func (f fakeConfig) IsStaging() bool    { return f.staging }

// Only prod spends the shared Sentry budget. Stage runs against parked
// platforms and absent upstreams, so letting it report costs quota to describe
// the environment — and a drained quota blinds prod, which is the env that
// matters.
func TestThrottleOnlyReportsFromProduction(t *testing.T) {
	cases := []struct {
		name string
		cfg  fakeConfig
		want bool // whether the event should survive BeforeSend
	}{
		{"production reports", fakeConfig{prod: true}, true},
		{"staging is silent", fakeConfig{staging: true}, false},
		{"development is silent", fakeConfig{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hook := throttle(tc.cfg)
			got := hook(&sentry.Event{Message: "boom"}, nil) != nil
			if got != tc.want {
				t.Errorf("event sent = %v, want %v", got, tc.want)
			}
		})
	}
}

// A nil config must not panic and must not send — the fail-closed direction.
func TestThrottleNilConfigIsSilent(t *testing.T) {
	if throttle(nil)(&sentry.Event{Message: "boom"}, nil) != nil {
		t.Error("nil config should drop events")
	}
}

// The cooldown is what keeps a flapping error from draining the month, so a
// repeat of the same message inside the window must not reach Sentry.
func TestThrottleCollapsesRepeatsInProduction(t *testing.T) {
	hook := throttle(fakeConfig{prod: true})
	if hook(&sentry.Event{Message: "same"}, nil) == nil {
		t.Fatal("first event should send")
	}
	if hook(&sentry.Event{Message: "same"}, nil) != nil {
		t.Error("repeat inside the cooldown window should drop")
	}
	if hook(&sentry.Event{Message: "different"}, nil) == nil {
		t.Error("a distinct message should send")
	}
}
