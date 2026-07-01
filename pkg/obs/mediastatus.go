package obs

import (
	"context"
	"errors"
	"log/slog"

	"github.com/andreykaipov/goobs/api/requests/mediainputs"
)

// Media states OBS reports for a media (ffmpeg) source. The full set is
// PLAYING / OPENING / BUFFERING / PAUSED / STOPPED / ENDED / ERROR / NONE.
const (
	MediaStatePlaying = "OBS_MEDIA_STATE_PLAYING"
	MediaStateEnded   = "OBS_MEDIA_STATE_ENDED"
	MediaStateStopped = "OBS_MEDIA_STATE_STOPPED"
	MediaStateError   = "OBS_MEDIA_STATE_ERROR"
	MediaStateNone    = "OBS_MEDIA_STATE_NONE"
)

// GetMediaInputState returns the OBS media state string for a media-source
// input (e.g. "OBS_MEDIA_STATE_PLAYING"). Opens a fresh connection per call,
// like the other helpers here. Returns ErrUnreachable wrapped with the dial
// error when OBS itself can't be reached, so callers can tell "OBS is down"
// apart from "the source isn't playing."
func GetMediaInputState(ctx context.Context, inputName string) (string, error) {
	client, err := dial(ctx)
	if err != nil {
		return "", errors.Join(ErrUnreachable, err)
	}
	defer func() {
		if err := client.Disconnect(); err != nil {
			slog.WarnContext(ctx, "obs disconnect", "err", err)
		}
	}()
	resp, err := client.MediaInputs.GetMediaInputStatus(
		mediainputs.NewGetMediaInputStatusParams().WithInputName(inputName),
	)
	if err != nil {
		return "", err
	}
	return resp.MediaState, nil
}

// MediaStateDown reports whether a media state means the source has stopped
// producing audio and will not recover on its own — the EOF-wedge (ENDED) and
// the hard-failure states. OPENING/BUFFERING are deliberately treated as
// transient (not down) so a normal reconnect doesn't trip the fallback.
func MediaStateDown(state string) bool {
	switch state {
	case MediaStateEnded, MediaStateStopped, MediaStateError, MediaStateNone:
		return true
	default:
		return false
	}
}
