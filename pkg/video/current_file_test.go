package video

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsMP4Path(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"uppercase .MP4", "/foo/bar/2018_0514_224801_013.MP4", true},
		{"lowercase .mp4", "/foo/bar/video.mp4", true},
		{"mixed case .Mp4", "video.Mp4", true},
		{"not an mp4", "/foo/bar/baz.txt", false},
		{"empty string", "", false},
		{"mp4 in middle of name", "mp4file.txt", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMP4Path(tt.in); got != tt.want {
				t.Fatalf("isMP4Path(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestReadPID(t *testing.T) {
	dir := t.TempDir()

	t.Run("valid pid", func(t *testing.T) {
		path := filepath.Join(dir, "valid.pid")
		if err := os.WriteFile(path, []byte("12345\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		pid, err := readPID(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pid != 12345 {
			t.Fatalf("got %d, want 12345", pid)
		}
	})

	t.Run("pid with surrounding whitespace", func(t *testing.T) {
		path := filepath.Join(dir, "whitespace.pid")
		if err := os.WriteFile(path, []byte("  9876  \n"), 0o644); err != nil {
			t.Fatal(err)
		}
		pid, err := readPID(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pid != 9876 {
			t.Fatalf("got %d, want 9876", pid)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if _, err := readPID(filepath.Join(dir, "nope.pid")); err == nil {
			t.Fatal("expected error for missing pidfile")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		path := filepath.Join(dir, "empty.pid")
		if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := readPID(path); err == nil {
			t.Fatal("expected error for empty pidfile")
		}
	})

	t.Run("non-numeric contents", func(t *testing.T) {
		path := filepath.Join(dir, "bad.pid")
		if err := os.WriteFile(path, []byte("not-a-pid\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := readPID(path); err == nil {
			t.Fatal("expected error for non-numeric pidfile")
		}
	})
}
