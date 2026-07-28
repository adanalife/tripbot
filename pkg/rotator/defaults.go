package rotator

import "slices"

// Side identifies which corner overlay a pool or budget belongs to. The values
// double as the JSON keys in a stored Config and as the console's editor tabs.
type Side string

const (
	SideLeft  Side = "left"
	SideRight Side = "right"
)

// Corner is the editable copy for one corner: the full command-hint pool, and
// the promo pool substituted on an instance where a hinted "!command" couldn't
// produce a result the viewer sees (see the promoMode gate in
// pkg/onscreens-server).
type Corner struct {
	Messages      []Message `json:"messages"`
	PromoMessages []Message `json:"promo_messages"`
}

// Config is the full editable rotator copy for one streaming platform — the
// payload stored per platform and pushed to that platform's onscreens-server.
//
// RareMessage is the left corner's 1-in-RareOdds easter egg; empty disables it.
// The live location/date lines are not represented here: they're generated from
// the currently-playing clip rather than authored, so onscreens-server mixes
// them into the promo pools at render time.
type Config struct {
	Left        Corner `json:"left"`
	Right       Corner `json:"right"`
	RareMessage string `json:"rare_message,omitempty"`
}

// DefaultRareMessage is the left corner's easter-egg line.
const DefaultRareMessage = "You found the rare message! Make a clip for a prize!"

// defaultLeftMessages are the command-hint lines for the left corner.
//
// !miles and !guess are Twitch-only (not in the YouTube command allowlist), so
// those lines are scoped to Twitch — a YouTube overlay would otherwise advertise
// commands that silently no-op there. Weight 2 makes !discord and !commands
// twice as likely as the unweighted lines.
//
// Lines parked rather than deleted, kept here so the ideas aren't lost now that
// the live list is edited in the admin console: "LEADER"; "Looking for artist
// for emotes and more"; "Twitch Prime subs keep us on air :D"; "Use !report to
// report stream issues".
var defaultLeftMessages = []Message{
	{Text: "Crave something new? Try `!timewarp`"},
	{Text: "Earn miles for every minute you watch (`!miles`)", Platforms: []string{PlatformTwitch}},
	{Text: "Follow the project elsewhere on `!socialmedia`"},
	{Text: "Join us on `!discord`", Weight: 2},
	{Text: "Try and `!guess` what state we're in", Platforms: []string{PlatformTwitch}},
	{Text: "Use `!commands` to interact with the bot", Weight: 2},
	{Text: "Where are we? (`!location`)"},
}

// defaultPromoLeftMessages replace the command-hint left pool in promoMode (a
// bot-less YouTube instance, or a read-only platform like TikTok/Instagram where
// the bot can't reply). The split between the corners is by action: this one owns
// "here is what you can make happen", the right owns "here is what you're
// watching".
//
// No line names a "!command", the same rule the pool has always followed: a hint
// is only worth a corner if it reaches someone who wouldn't act anyway, and a
// viewer who already knows the commands doesn't need telling. Gifting is the
// exception worth advertising because it's a platform UI action rather than
// something to type, and nobody would guess it does anything here.
//
// Mixed with the live location line at render time — the info !location returns.
var defaultPromoLeftMessages = []Message{
	{Text: "🎁 Send a gift → warp us to a random moment", Platforms: []string{PlatformTikTok}, Weight: 3},
	{Text: "Gift the stream, change what's on screen", Platforms: []string{PlatformTikTok}, Weight: 2},
	{Text: "Interactive chat is coming to YouTube soon", Platforms: []string{PlatformYouTube}},
	{Text: "Somewhere on a road across America"},
	{Text: "Every mile of this was actually driven"},
}

// defaultRightMessages are the command-hint lines for the right corner. All are
// platform-neutral (!location and !timewarp are both in the YouTube allowlist).
// Weight 2 makes the follow and !location hints twice as likely as the rest.
var defaultRightMessages = []Message{
	{Text: "Don't forget to follow :)", Weight: 2},
	{Text: "Try running `!location`", Weight: 2},
	{Text: "Try running `!timewarp`"},
	{Text: "Streaming 24 hours a day"},
}

// defaultPromoRightMessages replace the full command-hint right pool in
// promoMode (see defaultPromoLeftMessages). This corner owns "here is what
// you're watching" — the journey flavor plus each platform's own-platform call
// to action, worded in that platform's verb (YouTube subscribes, TikTok
// follows) so the two corners never advertise the same action at once.
//
// Mixed with the live date line at render time — the info !date returns.
var defaultPromoRightMessages = []Message{
	{Text: "Driving across America, 24 hours a day"},
	{Text: "Subscribe to ride along", Platforms: []string{PlatformYouTube}},
	{Text: "Follow to ride along", Platforms: []string{PlatformTikTok, PlatformInstagram}},
	{Text: "Slow-TV from the open road — just the drive"},
	{Text: "Real dashcam footage, streaming nonstop"},
}

// DefaultConfig returns the copy compiled into the binary — the fallback when a
// platform has no stored edits. Slices are deep-copied so a caller mutating the
// result (the console editor round-trip does) can't scribble on the defaults.
func DefaultConfig() Config {
	return Config{
		Left: Corner{
			Messages:      clone(defaultLeftMessages),
			PromoMessages: clone(defaultPromoLeftMessages),
		},
		Right: Corner{
			Messages:      clone(defaultRightMessages),
			PromoMessages: clone(defaultPromoRightMessages),
		},
		RareMessage: DefaultRareMessage,
	}
}

// DefaultConfigFor returns DefaultConfig with every pool reduced to the lines
// that apply to platform, and the Platforms scoping dropped. It's what the
// console prefills a fresh platform editor with: a YouTube editor opens without
// the Twitch-only !miles / !guess lines or TikTok's gift lines, rather than
// showing copy that would never render there. Stored configs are per-platform,
// so the scoping field has no meaning once a platform has been edited.
func DefaultConfigFor(platform string) Config {
	cfg := DefaultConfig()
	cfg.Left.Messages = filterFor(platform, cfg.Left.Messages)
	cfg.Left.PromoMessages = filterFor(platform, cfg.Left.PromoMessages)
	cfg.Right.Messages = filterFor(platform, cfg.Right.Messages)
	cfg.Right.PromoMessages = filterFor(platform, cfg.Right.PromoMessages)
	return cfg
}

// filterFor keeps the messages applicable to platform and clears the now-moot
// Platforms scoping on each.
func filterFor(platform string, msgs []Message) []Message {
	out := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		if !m.AppliesTo(platform) {
			continue
		}
		m.Platforms = nil
		out = append(out, m)
	}
	return out
}

// clone deep-copies a pool, Platforms slices included, so a caller can mutate
// the result freely without reaching back into the package-level defaults.
func clone(msgs []Message) []Message {
	out := make([]Message, len(msgs))
	for i, m := range msgs {
		if m.Platforms != nil {
			m.Platforms = slices.Clone(m.Platforms)
		}
		out[i] = m
	}
	return out
}
