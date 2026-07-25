package onscreensServer

import (
	c "github.com/adanalife/tripbot/pkg/config/onscreens-server"
	"time"
)

var leftRotatorUpdateFrequency = time.Duration(45 * time.Second)

// !miles and !guess are Twitch-only (not in the YouTube command allowlist), so
// those lines are scoped to Twitch — a YouTube overlay would otherwise advertise
// commands that silently no-op there. Weight 2 makes !discord and !commands
// twice as likely as the unweighted lines.
var possibleLeftMessages = []rotatorMessage{
	{Text: "Crave something new? Try `!timewarp`"},
	{Text: "Earn miles for every minute you watch (`!miles`)", Platforms: []string{platformTwitch}},
	{Text: "Follow the project elsewhere on `!socialmedia`"},
	{Text: "Join us on `!discord`", Weight: 2},
	{Text: "Try and `!guess` what state we're in", Platforms: []string{platformTwitch}},
	{Text: "Use `!commands` to interact with the bot", Weight: 2},
	{Text: "Where are we? (`!location`)"},
	// {Text: "LEADER"},
	// {Text: "Looking for artist for emotes and more"},
	// {Text: "Twitch Prime subs keep us on air :D"},
	// {Text: "Use !report to report stream issues"},
}

// botlessLeftMessages replace the command-hint left rotator in promoMode (a
// bot-less YouTube instance, or a read-only platform like TikTok/Instagram where
// the bot can't reply): no chat command lands a visible reply there, so this
// corner points viewers at the live, interactive Twitch stream — the "where to
// interact" half of the split. The right corner carries the subscribe + journey
// lines, so the two corners never echo each other. Teases the guess/miles
// features without printing a "!command" token a viewer would type into an
// unread/unanswered chat. On a promoMode stream these are mixed with the live
// location line (see leftLiveLine) — the info the !location command would
// return. The "coming to YouTube" tease is scoped to YouTube (it names that
// platform); every other line is platform-neutral and safe on any promoMode
// platform.
var botlessLeftMessages = []rotatorMessage{
	{Text: "Chat live with the bot on Twitch", Weight: 2},
	{Text: "twitch.tv/ADanaLife_", Weight: 2},
	{Text: "Interactive chat is coming to YouTube soon", Platforms: []string{platformYouTube}},
	{Text: "Want to talk to the bot? It's live on Twitch"},
	{Text: "Guess the state and earn miles — live on Twitch"},
}

// liveDataWeight biases the live location/date line over the static promo lines
// in the bot-less pools — the data is the headline (it's what the !location /
// !date commands would return), the promo is the remainder. Tunable; ~50-65%
// data against the current promo weights.
const liveDataWeight = 6

// leftLiveLine is the bot-less left-rotator live-data line: the current location
// ("📍 City, State") when tripbot has pushed a fresh one. Paired with
// rightLiveLine's date so the two corners show "where" and "when" rather than
// duplicating one field.
func leftLiveLine(now time.Time) (rotatorMessage, bool) {
	if loc, _, ok := liveLocation.snapshot(now); ok && loc != "" {
		return rotatorMessage{Text: "📍 " + loc, Weight: liveDataWeight}, true
	}
	return rotatorMessage{}, false
}

// newLeftRotator configures the left corner. The caller pairs it with the right
// rotator and calls start().
func newLeftRotator(cfg *c.OnscreensServerConfig) *rotator {
	return &rotator{
		cfg:             cfg,
		kind:            "left-rotator",
		freq:            leftRotatorUpdateFrequency,
		messages:        possibleLeftMessages,
		botlessMessages: botlessLeftMessages,
		liveLine:        leftLiveLine,
		rareMessage:     "You found the rare message! Make a clip for a prize!",
	}
}
