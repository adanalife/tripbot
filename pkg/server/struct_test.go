package server

import "testing"

// New gives a Server with the default "dev" version tag.
func TestNewServerDefaults(t *testing.T) {
	s := New()
	if s.versionTag != "dev" {
		t.Errorf("versionTag = %q, want %q", s.versionTag, "dev")
	}
}

// SetVersion mutates the instance it's called on, and two Servers hold
// independent version state. SetVersion ignores an empty string.
func TestServerSetVersion(t *testing.T) {
	a := New()
	b := New()

	a.SetVersion("v1.2.3")
	if a.versionTag != "v1.2.3" {
		t.Errorf("a.versionTag = %q, want %q", a.versionTag, "v1.2.3")
	}
	if b.versionTag != "dev" {
		t.Errorf("b picked up a's state: version=%q", b.versionTag)
	}

	a.SetVersion("")
	if a.versionTag != "v1.2.3" {
		t.Errorf("SetVersion(\"\") clobbered the tag: %q", a.versionTag)
	}
}
