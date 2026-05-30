package video

import (
	"os"
	"path/filepath"
	"testing"
)

// makeSymlinkOrSkip creates a symlink and skips the test if symlinks
// aren't supported by the filesystem (rare but not impossible in CI).
func makeSymlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported on this filesystem: %v", err)
	}
}

func TestFindOpenMP4InFDDir(t *testing.T) {
	t.Run("finds MP4 by symlink target basename", func(t *testing.T) {
		dir := t.TempDir()
		makeSymlinkOrSkip(t, "/dev/null", filepath.Join(dir, "0"))
		makeSymlinkOrSkip(t, "/dev/null", filepath.Join(dir, "1"))
		makeSymlinkOrSkip(t, "socket:[12345]", filepath.Join(dir, "2"))
		makeSymlinkOrSkip(t, "/data/dashcam/2018_0514_224801_013.MP4", filepath.Join(dir, "7"))

		got, err := findOpenMP4InFDDir(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "2018_0514_224801_013.MP4"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("returns error when no MP4 is open", func(t *testing.T) {
		dir := t.TempDir()
		makeSymlinkOrSkip(t, "/dev/null", filepath.Join(dir, "0"))
		makeSymlinkOrSkip(t, "/etc/hosts", filepath.Join(dir, "3"))

		if _, err := findOpenMP4InFDDir(dir); err == nil {
			t.Fatal("expected error when no MP4 is open")
		}
	})

	t.Run("returns error when fd directory is missing", func(t *testing.T) {
		if _, err := findOpenMP4InFDDir("/nonexistent/proc/99999999/fd"); err == nil {
			t.Fatal("expected error for missing fd directory")
		}
	})

	t.Run("matches lowercase .mp4 extension", func(t *testing.T) {
		dir := t.TempDir()
		makeSymlinkOrSkip(t, "/some/path/clip.mp4", filepath.Join(dir, "4"))

		got, err := findOpenMP4InFDDir(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "clip.mp4" {
			t.Fatalf("got %q, want %q", got, "clip.mp4")
		}
	})
}
