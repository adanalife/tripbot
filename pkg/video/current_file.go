package video

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/adanalife/tripbot/pkg/helpers"
)

// pidFilePath returns the PID file used to locate the process whose open
// files we inspect for the currently-playing video. The OS split mirrors
// the legacy bin/current-file.sh behavior: linux watches VLC, darwin
// watches OBS.
func pidFilePath() string {
	if helpers.RunningOnDarwin() {
		return filepath.Join(helpers.ProjectRoot(), "run", "OBS.pid")
	}
	return filepath.Join(helpers.ProjectRoot(), "run", "VLC.pid")
}

// readPID reads the PID from the given pidfile and returns it as an int.
func readPID(path string) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	pidStr := strings.TrimSpace(string(raw))
	if pidStr == "" {
		return 0, errors.New("pidfile is empty")
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, err
	}
	return pid, nil
}

// isMP4Path reports whether the given path looks like an MP4 file
// (case-insensitive .mp4 suffix). Matches the `grep -i '\.MP4'` filter
// in the legacy bin/current-file.sh.
func isMP4Path(p string) bool {
	return strings.HasSuffix(strings.ToLower(p), ".mp4")
}

// currentVideoFile reads the PID file, finds the first open .MP4 file
// belonging to that process, and returns its basename. Replaces the
// legacy bin/current-file.sh shell-out.
func currentVideoFile() (string, error) {
	pid, err := readPID(pidFilePath())
	if err != nil {
		return "", err
	}
	return findOpenMP4(pid)
}
