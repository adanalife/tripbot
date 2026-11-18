package leaderboardEvents

import "fmt"

// domain is the fixed segment between <env> and the verb in every leaderboard
// command subject: tripbot.<env>.leaderboard.<verb>. Deliberately not the
// "onscreens" domain — that namespace belongs to onscreens-server's own
// registry, and this command is consumed by tripbot.
const domain = "leaderboard"

// Platform values for the per-platform subject leaf. Each per-platform tripbot
// (tripbot-twitch / tripbot-youtube) drives its own overlays, so a leaderboard
// command is scoped to one platform and only that instance handles it.
const (
	PlatformTwitch  = "twitch"
	PlatformYouTube = "youtube"
)

// ShowSubject builds tripbot.<env>.leaderboard.show.<platform> — the operator
// "put this board on <platform>'s stream" command. The per-platform tripbot
// instance subscribes to its own leaf, fetches the named board and publishes
// the onscreens command that renders it. It's the console-published counterpart
// to the !leaderboard family of chat commands, without the chat line.
func ShowSubject(env, platform string) string {
	return fmt.Sprintf("tripbot.%s.%s.show.%s", env, domain, platform)
}
