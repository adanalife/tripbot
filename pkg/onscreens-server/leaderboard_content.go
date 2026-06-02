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
func renderLeaderboard(title string, leaderboard [][]string) string {
	size := 5
	if len(leaderboard) < size {
		size = len(leaderboard)
	}
	leaderboard = leaderboard[:size]

	var b strings.Builder
	b.WriteString(`<div class="lb-grid">`)
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
