package telemetry

import (
	"context"
	"testing"
	"time"

	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func TestInit_DisabledViaEnv(t *testing.T) {
	t.Setenv("OTEL_SDK_DISABLED", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://example.invalid")

	shutdown, err := Init(context.Background(), "test", "0.0.0")
	if err != nil {
		t.Fatalf("Init() unexpected error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("Init() returned nil shutdown func")
	}

	// Shutdown should be safe to call and quick when disabled.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown() unexpected error: %v", err)
	}
}

func TestInit_DisabledViaMissingEndpoint(t *testing.T) {
	t.Setenv("OTEL_SDK_DISABLED", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	shutdown, err := Init(context.Background(), "test", "0.0.0")
	if err != nil {
		t.Fatalf("Init() unexpected error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("Init() returned nil shutdown func")
	}

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() unexpected error: %v", err)
	}
}

// TestResourceCarriesInstanceID pins the attribute that gives each instance its
// own series. Drop it and every metric recorded without an explicit platform
// attribute collapses onto one shared series across all the per-platform pods,
// reporting whichever of them pushed last.
func TestResourceCarriesInstanceID(t *testing.T) {
	t.Setenv("HOSTNAME", "tripbot-twitch-8b446df8c-hrc6s")

	res, err := newResource(t.Context(), "tripbot", "0.0.0")
	if err != nil {
		t.Fatalf("newResource() unexpected error: %v", err)
	}

	for _, attr := range res.Attributes() {
		if attr.Key == semconv.ServiceInstanceIDKey {
			if got := attr.Value.AsString(); got != "tripbot-twitch-8b446df8c-hrc6s" {
				t.Fatalf("service.instance.id = %q, want the pod name from HOSTNAME", got)
			}
			return
		}
	}
	t.Fatal("resource carries no service.instance.id; per-instance series would collide")
}

func TestDisabled(t *testing.T) {
	cases := []struct {
		name        string
		sdkDisabled string
		endpoint    string
		want        bool
	}{
		{"flag-true", "true", "http://example.invalid", true},
		{"flag-1", "1", "http://example.invalid", true},
		{"flag-yes", "yes", "http://example.invalid", true},
		{"flag-cased", "TRUE", "http://example.invalid", true},
		{"no-endpoint", "", "", true},
		{"endpoint-only", "", "http://example.invalid", false},
		{"flag-false-with-endpoint", "false", "http://example.invalid", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OTEL_SDK_DISABLED", tc.sdkDisabled)
			t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", tc.endpoint)
			if got := disabled(); got != tc.want {
				t.Fatalf("disabled() = %v, want %v", got, tc.want)
			}
		})
	}
}
