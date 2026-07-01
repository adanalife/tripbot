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

// SetInputLocalFileMode flips an ffmpeg_source to play a local file by setting
// is_local_file=true and pointing local_file at the given path. The overlay
// merge leaves the source's `input` (network URL) field untouched, so
// SetInputNetworkMode can flip straight back to it without re-supplying the
// URL. Used by the audio-fallback watchdog to swap the SomaFM source onto the
// local Car Hum bed when SomaFM is unreachable. file must exist inside the OBS
// container.
//
// looping=true is essential: the SomaFM source ships with looping unset (a
// radio stream doesn't loop), so without forcing it on, the fallback FLAC would
// play exactly once and then go silent — re-creating the dead air we're trying
// to avoid. (Confirmed on stage 2026-06-24.)
func SetInputLocalFileMode(ctx context.Context, inputName, file string) error {
	return setInputSettings(ctx, inputName, map[string]any{
		"is_local_file": true,
		"local_file":    file,
		"looping":       true,
	})
}

// SetInputNetworkMode flips an ffmpeg_source back to its network stream by
// setting is_local_file=false (and clearing looping, which is meaningless for a
// live stream). The source's `input` URL is preserved across the local-file
// detour (the overlay merge never overwrote it), so OBS reopens the original
// stream. The inverse of SetInputLocalFileMode.
func SetInputNetworkMode(ctx context.Context, inputName string) error {
	return setInputSettings(ctx, inputName, map[string]any{
		"is_local_file": false,
		"looping":       false,
	})
}

// setInputSettings is the shared overlay-merge SetInputSettings call behind the
// mode-swap helpers: open a fresh connection, merge the given settings onto the
// named input, disconnect.
func setInputSettings(ctx context.Context, inputName string, settings map[string]any) error {
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
			WithInputSettings(settings).
			WithOverlay(true),
	)
	return err
}
