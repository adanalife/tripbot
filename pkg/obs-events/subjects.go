package obsEvents

import "fmt"

// domain is the fixed segment between <env> and the verb in every obs command
// subject: tripbot.<env>.obs.<verb>.
const domain = "obs"

// Platform values for the per-platform subject leaf. Each per-platform tripbot
// (tripbot-twitch / tripbot-youtube) owns its own OBS WebSocket connection, so
// an obs command is scoped to one platform and only that instance handles it.
const (
	PlatformTwitch  = "twitch"
	PlatformYouTube = "youtube"
)

// RefreshSubject builds tripbot.<env>.obs.refresh.<platform> — the operator
// "hard-reload every OBS browser source on <platform>'s OBS" command. The
// per-platform tripbot instance subscribes to its own leaf and calls
// pkg/obs.RefreshBrowserSources. It's the console-published counterpart to the
// !refreshoverlays chat command: the recovery for a crashed/frozen overlay a
// soft refresh can't revive.
func RefreshSubject(env, platform string) string {
	return fmt.Sprintf("tripbot.%s.%s.refresh.%s", env, domain, platform)
}
