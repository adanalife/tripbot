package twitch

import (
	"os/exec"
	"path"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
)

// SetStreamTags will call a shell script (lol) to set stream tags
//TODO: use the twitch API instead of a shell script when possible
func SetStreamTags() {
	// run the shell script to get set stream tags
	scriptPath := path.Join(helpers.ProjectRoot(), "bin/set-tags.sh")
	_, err := exec.Command(scriptPath, c.Conf.ExternalURL).Output()
	if err != nil {
		terrors.Log(err, "failed to run script")
		return
	}
	return
}
