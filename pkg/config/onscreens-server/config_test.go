package config

import (
	"fmt"
	"testing"

	"github.com/adanalife/tripbot/pkg/contract"
)

// The listener's port is the contract's, not a literal spelled in a struct tag
// — a rename there has to reach the binary, which is the direction
// pkg/contract exists to hold. An explicit bind address still wins.
func TestLoadBindAddressDefaultsToTheContract(t *testing.T) {
	t.Setenv("ONSCREENS_SERVER_BIND_ADDRESS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if want := fmt.Sprintf(":%d", contract.PortOnscreensHTTP); cfg.OnscreensServerBindAddress != want {
		t.Errorf("bind address = %q, want %q", cfg.OnscreensServerBindAddress, want)
	}

	t.Setenv("ONSCREENS_SERVER_BIND_ADDRESS", "127.0.0.1:9999")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.OnscreensServerBindAddress != "127.0.0.1:9999" {
		t.Errorf("bind address = %q, want %q", cfg.OnscreensServerBindAddress, "127.0.0.1:9999")
	}
}
