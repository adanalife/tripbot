package obs

import (
	"context"
	"log/slog"

	"github.com/andreykaipov/goobs/api/requests/inputs"
)

// SetBackgroundAudioFile repoints an ffmpeg_source input at a different local
// media file over the OBS WebSocket — used by the !carsound command to swap the
// YouTube background drone live without reloading the scene collection. The
// `overlay=true` merge means only local_file changes; looping/volume/etc. on
// the source are left intact. inputName is the OBS source name (e.g. "Car Hum")
// and file is an absolute path that must exist inside the OBS container.
//
// Opens a fresh connection per call (toggles are rare), same as the Start/Stop
// helpers. The change is in-memory in OBS and resets to the scene default on
// the next OBS restart — fine for an ephemeral, fun command.
func SetBackgroundAudioFile(ctx context.Context, inputName, file string) error {
	client, err := dial(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err := client.Disconnect(); err != nil {
			slog.WarnContext(ctx, "obs disconnect", "err", err)
		}
	}()

	_, err = client.Inputs.SetInputSettings(
		inputs.NewSetInputSettingsParams().
			WithInputName(inputName).
			WithInputSettings(map[string]any{"local_file": file}).
			WithOverlay(true),
	)
	return err
}
