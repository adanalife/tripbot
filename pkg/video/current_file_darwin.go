package video

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// findOpenMP4 shells out to lsof for the given pid and parses its output
// for the first .MP4 file. macOS doesn't expose /proc, so lsof is the
// portable path here.
func findOpenMP4(pid int) (string, error) {
	out, err := exec.Command("lsof", "-p", fmt.Sprintf("%d", pid)).Output()
	if err != nil {
		return "", fmt.Errorf("lsof failed: %w", err)
	}
	return parseLsofForMP4(string(out))
}

// parseLsofForMP4 scans `lsof -p PID` output for the first line that
// looks like an open MP4 file and returns the basename of the path. The
// NAME column is the last whitespace-separated field of an lsof line.
func parseLsofForMP4(lsofOutput string) (string, error) {
	for _, line := range strings.Split(lsofOutput, "\n") {
		if !isMP4Path(line) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		// The NAME column is the last field. lsof can emit paths with
		// spaces, but we only need files matching the dashcam naming
		// convention (which has none), so the last-field heuristic is
		// safe.
		path := fields[len(fields)-1]
		if isMP4Path(path) {
			return filepath.Base(path), nil
		}
	}
	return "", errors.New("no MP4 found in lsof output")
}
