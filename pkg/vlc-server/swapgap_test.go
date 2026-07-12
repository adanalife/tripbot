package vlcServer

import (
	"testing"
	"time"
)

func TestSwapGapTracker(t *testing.T) {
	var tr swapGapTracker
	start := time.Now()

	if _, ok := tr.observe(start); ok {
		t.Fatal("observe on an unarmed tracker should report ok=false")
	}

	tr.arm(start)
	gap, ok := tr.observe(start.Add(250 * time.Millisecond))
	if !ok {
		t.Fatal("observe after arm should report ok=true")
	}
	if gap < 0.249 || gap > 0.251 {
		t.Fatalf("gap = %v, want ~0.25", gap)
	}

	if _, ok := tr.observe(start.Add(time.Second)); ok {
		t.Fatal("observe should disarm the tracker")
	}

	tr.arm(start)
	tr.disarm()
	if _, ok := tr.observe(start.Add(time.Second)); ok {
		t.Fatal("disarm should clear the armed state")
	}
}
