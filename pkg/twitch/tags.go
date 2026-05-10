package twitch

import (
	"context"
	"os/exec"
	"path/filepath"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	terrors "github.com/adanalife/tripbot/pkg/errors"
	"github.com/adanalife/tripbot/pkg/helpers"
)

// SetStreamTags will call a shell script (lol) to set stream tags.
//
// The ctx is accepted for trace propagation; it's threaded into the exec
// call so the shell script invocation respects scheduler-level cancellation.
//
//TODO: use the twitch API instead of a shell script when possible
func SetStreamTags(ctx context.Context) {
	if helpers.RunningOnWindows() {
		terrors.Log(nil, "can't run script on Windows")
	}
	// run the shell script to get set stream tags
	scriptPath := filepath.Join(helpers.ProjectRoot(), "bin", "set-tags.sh")
	_, err := exec.CommandContext(ctx, scriptPath, c.Conf.ExternalURL).Output()
	if err != nil {
		terrors.Log(err, "failed to run script")
		return
	}
	return
}
