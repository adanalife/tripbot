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

// promoLeftMessages replace the full command-hint left rotator in promoMode (a
// bot-less YouTube instance, or a read-only platform like TikTok/Instagram
// where the bot can't reply). The split between the corners is by action: this
// one owns "here is what you can make happen", the right owns "here is what
// you're watching".
//
// On a read-only platform the effect commands are the ones worth hinting —
// !timewarp and !find land their result on the stream, which needs no chat
// reply — so those lines are scoped to TikTok, alongside the gift copy. A
// reply-only command (!location, !miles) is deliberately absent: its whole
// result is a chat line nobody would see. The YouTube tease names its own
// platform and is scoped to it; everything unscoped is safe anywhere.
//
// On a promoMode stream these are mixed with the live location line (see
// leftLiveLine) — the info the !location command would return.
var promoLeftMessages = []rotatorMessage{
	{Text: "🎁 Send a gift → warp us to a random moment", Platforms: []string{platformTikTok}, Weight: 3},
	{Text: "Gift the stream, change what's on screen", Platforms: []string{platformTikTok}},
	{Text: "Type `!timewarp` to jump somewhere new", Platforms: []string{platformTikTok}, Weight: 2},
	{Text: "`!find a tunnel at sunset` — we'll go there", Platforms: []string{platformTikTok}, Weight: 2},
	{Text: "Interactive chat is coming to YouTube soon", Platforms: []string{platformYouTube}},
	{Text: "Somewhere on a road across America"},
	{Text: "Every mile of this was actually driven"},
}

// liveDataWeight biases the live location/date line over the static promo lines
// in the promo pools — the data is the headline (it's what the !location /
// !date commands would return), the promo is the remainder. Tunable; ~50-65%
// data against the current promo weights.
const liveDataWeight = 6

// leftLiveLine is the promoMode left-rotator live-data line: the current location
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
		cfg:           cfg,
		kind:          "left-rotator",
		freq:          leftRotatorUpdateFrequency,
		messages:      possibleLeftMessages,
		promoMessages: promoLeftMessages,
		liveLine:      leftLiveLine,
		rareMessage:   "You found the rare message! Make a clip for a prize!",
	}
}
