package vlcServer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writeTempFile(t *testing.T, name string, size int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	return path
}

func TestWarmCacheReadsFileWithoutError(t *testing.T) {
	path := writeTempFile(t, "clip.MP4", 4096)
	// Should complete without panic/error for a readable file (whole-file
	// read) and for a bounded read.
	warmCache(context.Background(), path, 0)
	warmCache(context.Background(), path, 1024)
}

func TestWarmCacheMissingFileIsNoop(t *testing.T) {
	// A missing file must not panic — warming is best-effort.
	warmCache(context.Background(), "/nonexistent/clip.MP4", 0)
}

func TestRefreshWarmRandomSetsInRangeIndex(t *testing.T) {
	s := &Server{
		warmRandomIdx: -1,
		VideoPaths: []string{
			writeTempFile(t, "a.MP4", 16),
			writeTempFile(t, "b.MP4", 16),
			writeTempFile(t, "c.MP4", 16),
		},
	}
	s.refreshWarmRandom(context.Background())

	s.primeMu.Lock()
	idx := s.warmRandomIdx
	s.primeMu.Unlock()

	if idx < 0 || idx >= len(s.VideoPaths) {
		t.Fatalf("warmRandomIdx = %d, want in [0,%d)", idx, len(s.VideoPaths))
	}
}

func TestRefreshWarmRandomEmptyCorpusLeavesNoneReady(t *testing.T) {
	s := &Server{warmRandomIdx: -1}
	s.refreshWarmRandom(context.Background())

	s.primeMu.Lock()
	idx := s.warmRandomIdx
	s.primeMu.Unlock()

	if idx != -1 {
		t.Fatalf("warmRandomIdx = %d, want -1 (nothing to warm)", idx)
	}
}

func TestEnsureWarmRandomKeepsExistingReady(t *testing.T) {
	s := &Server{
		warmRandomIdx: 1,
		VideoPaths:    []string{writeTempFile(t, "a.MP4", 16), writeTempFile(t, "b.MP4", 16)},
	}
	s.ensureWarmRandom(context.Background())

	s.primeMu.Lock()
	idx := s.warmRandomIdx
	s.primeMu.Unlock()

	if idx != 1 {
		t.Fatalf("warmRandomIdx = %d, want 1 (should not refresh when one is ready)", idx)
	}
}
