package twitch

import (
	"os/exec"
	"path/filepath"

	"github.com/adanalife/tripbot/pkg/config"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
)

// SetStreamTags will call a shell script (lol) to set stream tags
//TODO: use the twitch API instead of a shell script when possible
func SetStreamTags() {
	if helpers.RunningOnWindows() {
		terrors.Log(nil, "can't run script on Windows")
	}
	// run the shell script to get set stream tags
	scriptPath := filepath.Join(helpers.ProjectRoot(), "bin", "set-tags.sh")
	_, err := exec.Command(scriptPath, config.ExternalURL).Output()
	if err != nil {
		terrors.Log(err, "failed to run script")
		return
	}
	return
}
