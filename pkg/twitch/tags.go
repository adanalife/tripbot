package twitch

import (
	"os/exec"
	"path"

	terrors "github.com/dmerrick/danalol-stream/pkg/errors"
	"github.com/dmerrick/danalol-stream/pkg/helpers"
)

// SetStreamTags will call a shell script (lol) to set stream tags
//TODO: use the twitch API instead of a shell script when possible
func SetStreamTags() {
	// run the shell script to get set stream tags
	scriptPath := path.Join(helpers.ProjectRoot(), "bin/set-tags.sh")
	out, err := exec.Command(scriptPath).Output()
	//TODO: remove me
	helpers.SendSMS("set stream tags!")
	if err != nil {
		terrors.Log(err, "failed to run script")
		return
	}
	return
}
