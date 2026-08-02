package onscreensServer

import (
	"fmt"
	"html"
	"strings"
)

// renderLeaderboard renders the leaderboard onscreen as a CSS-grid HTML
// fragment. The score column auto-sizes to the widest entry via
// grid-template-columns, so digits line up across rows regardless of font.
// The leaderboard onscreen is registered with RenderAsHTML so the
// browser-source template injects this via innerHTML.
//
// Moved here from pkg/users.LeaderboardContent: onscreens-server now owns
// presentation, so the wire (NATS + HTTP) carries structured {title, rows}
// rather than a pre-rendered blob, and the renderer lives next to the
// overlay it feeds. Kept dependency-free (fmt/html/strings) so it doesn't
// drag pkg/users' DB/config init into this binary.
// maxLeaderboardRows is what fits on the overlay. It matches the chatbot's
// leaderboardSize — the sender already caps its boards at ten, so anything
// stricter here silently drops rows a viewer was told about in chat. Restated
// rather than imported: a shared package must not pull in the chatbot, and the
// event arrives over NATS from a sender this binary can't bound.
const maxLeaderboardRows = 10

func renderLeaderboard(title string, leaderboard [][]string) string {
	leaderboard = leaderboard[:min(len(leaderboard), maxLeaderboardRows)]

	var b strings.Builder
	b.WriteString(`<div class="lb-grid">`)
	// strings.Title is deprecated, but x/text's cases.Title would lower-case the
	// rest of each word and treat digits as boundaries — needless risk for a
	// title the chatbot already sends capitalised ("Total Miles", "July Miles").
	//nolint:staticcheck // SA1019: see above
	fmt.Fprintf(&b, `<div class="lb-title">%s</div>`, html.EscapeString(strings.Title(title)))
	for _, row := range leaderboard {
		fmt.Fprintf(
			&b,
			`<span class="lb-score">%s</span><span class="lb-user">(%s)</span>`,
			html.EscapeString(row[1]),
			html.EscapeString(row[0]),
		)
	}
	b.WriteString(`</div>`)
	return b.String()
}
