package obs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	goobs "github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/requests/inputs"
)

// browserSourceKind is the OBS input kind for a CEF browser_source.
const browserSourceKind = "browser_source"

// blankURL is the transient page a browser source is pointed at to force
// obs-browser to tear down and recreate its CEF render process.
const blankURL = "about:blank"

// browserReloadGap is how long a source sits on blankURL before its real url is
// restored — long enough for obs-browser to actually drop the old render
// process rather than coalescing the two settings writes into a no-op.
const browserReloadGap = 600 * time.Millisecond

// RefreshBrowserSources hard-reloads every browser_source in OBS by swapping its
// url to about:blank and back, forcing obs-browser to recreate the CEF render
// process. Returns the number of sources reloaded.
//
// This is the recovery the hourly soft refresh can't do: a browser source whose
// webpage has crashed (CEF "Webpage has crashed unexpectedly") is NOT revived by
// PressInputPropertiesButton("refreshnocache") — only re-setting the url
// respawns the render process. Reloads all sources on one connection; a
// per-source failure is logged and skipped so one bad source doesn't strand the
// rest.
func RefreshBrowserSources(ctx context.Context) (int, error) {
	client, err := dial(ctx)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := client.Disconnect(); err != nil {
			slog.WarnContext(ctx, "obs disconnect", "err", err)
		}
	}()

	list, err := client.Inputs.GetInputList(inputs.NewGetInputListParams())
	if err != nil {
		return 0, fmt.Errorf("obs get input list: %w", err)
	}

	var reloaded int
	for _, in := range list.Inputs {
		if in.InputKind != browserSourceKind {
			continue
		}
		name := in.InputName
		settings, err := client.Inputs.GetInputSettings(inputs.NewGetInputSettingsParams().WithInputName(name))
		if err != nil {
			slog.WarnContext(ctx, "obs get input settings", "err", err, "input", name)
			continue
		}
		// A browser source backed by a local file (not a url) has nothing to
		// reload this way; skip it rather than clobbering its settings.
		url, ok := settings.InputSettings["url"].(string)
		if !ok || url == "" {
			continue
		}
		if err := setBrowserURL(client, name, blankURL); err != nil {
			slog.WarnContext(ctx, "obs blank browser url", "err", err, "input", name)
			continue
		}
		time.Sleep(browserReloadGap)
		if err := setBrowserURL(client, name, url); err != nil {
			// Left on about:blank — log loudly; the source is blank until the next
			// refresh restores it.
			slog.ErrorContext(ctx, "obs restore browser url", "err", err, "input", name, "url", url)
			continue
		}
		reloaded++
	}
	slog.InfoContext(ctx, "hard-refreshed obs browser sources", "count", reloaded)
	return reloaded, nil
}

// setBrowserURL merges a new url onto a browser source over an existing
// connection (overlay=true leaves the source's other settings intact).
func setBrowserURL(client *goobs.Client, name, url string) error {
	_, err := client.Inputs.SetInputSettings(
		inputs.NewSetInputSettingsParams().
			WithInputName(name).
			WithInputSettings(map[string]any{"url": url}).
			WithOverlay(true),
	)
	return err
}

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
