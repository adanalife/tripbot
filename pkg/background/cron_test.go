package background

import (
	"testing"
	"time"

	"github.com/go-co-op/gocron/v2"
)

func TestNewStartStop(t *testing.T) {
	s, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	if s == nil {
		t.Fatal("New() returned nil scheduler")
	}

	ran := make(chan struct{}, 1)
	if _, err := s.NewJob(
		gocron.DurationJob(10*time.Millisecond),
		gocron.NewTask(func() {
			select {
			case ran <- struct{}{}:
			default:
			}
		}),
	); err != nil {
		t.Fatalf("NewJob returned error: %v", err)
	}

	s.Start()
	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not run within 2s of Start")
	}

	if err := s.Stop(); err != nil {
		t.Fatalf("Stop() returned error: %v", err)
	}
}

// Stop is safe before Start and on a nil scheduler so shutdown paths can call
// it unconditionally.
func TestStopIsNilSafe(t *testing.T) {
	var s *Scheduler
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop() on nil receiver returned error: %v", err)
	}

	fresh, err := New()
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	if err := fresh.Stop(); err != nil {
		t.Fatalf("Stop() before Start returned error: %v", err)
	}
}
