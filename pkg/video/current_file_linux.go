package video

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// findOpenMP4 finds the first MP4 file open by the given process by
// walking /proc/<pid>/fd and resolving each fd's symlink. Returns the
// basename of the first match.
func findOpenMP4(pid int) (string, error) {
	return findOpenMP4InFDDir(fmt.Sprintf("/proc/%d/fd", pid))
}

// findOpenMP4InFDDir walks an fd directory (in the shape of
// /proc/<pid>/fd: a directory of symlinks pointing at open files) and
// returns the basename of the first symlink target that looks like an
// MP4. Split out from findOpenMP4 so it can be unit-tested without
// needing a real /proc entry.
func findOpenMP4InFDDir(fdDir string) (string, error) {
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join(fdDir, entry.Name()))
		if err != nil {
			// skip fds we can't read (e.g. sockets that vanished mid-walk)
			continue
		}
		if isMP4Path(target) {
			return filepath.Base(target), nil
		}
	}
	return "", errors.New("no MP4 found in process's open files")
}
