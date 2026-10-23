package obs

// Media states OBS reports for a media (ffmpeg) source. The full set is
// PLAYING / OPENING / BUFFERING / PAUSED / STOPPED / ENDED / ERROR / NONE.
const (
	MediaStatePlaying = "OBS_MEDIA_STATE_PLAYING"
	MediaStateEnded   = "OBS_MEDIA_STATE_ENDED"
	MediaStateStopped = "OBS_MEDIA_STATE_STOPPED"
	MediaStateError   = "OBS_MEDIA_STATE_ERROR"
	MediaStateNone    = "OBS_MEDIA_STATE_NONE"
)

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
