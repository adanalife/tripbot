package vlcServer

import "testing"

func TestNextIndex(t *testing.T) {
	tests := []struct {
		name             string
		current, offset  int
		length           int
		want             int
	}{
		{"forward in range", 2, 1, 10, 3},
		{"forward wraps to start", 9, 1, 10, 0},
		{"forward multiple wraps", 0, 25, 10, 5},
		{"backward in range", 5, -2, 10, 3},
		{"backward wraps to end", 0, -1, 10, 9},
		{"backward multiple wraps", 0, -25, 10, 5},
		{"zero offset returns current", 7, 0, 10, 7},
		{"length 1 always returns 0", 0, 5, 1, 0},
		{"empty length returns 0", 0, 5, 0, 0},
		{"negative length returns 0", 5, 1, -1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextIndex(tt.current, tt.offset, tt.length)
			if got != tt.want {
				t.Fatalf("nextIndex(%d, %d, %d) = %d, want %d",
					tt.current, tt.offset, tt.length, got, tt.want)
			}
		})
	}
}

func TestNextIndexAlwaysInRange(t *testing.T) {
	length := 7
	for current := 0; current < length; current++ {
		for offset := -20; offset <= 20; offset++ {
			got := nextIndex(current, offset, length)
			if got < 0 || got >= length {
				t.Fatalf("nextIndex(%d, %d, %d) = %d, out of range [0,%d)",
					current, offset, length, got, length)
			}
		}
	}
}
