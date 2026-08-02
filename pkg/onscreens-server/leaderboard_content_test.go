package onscreensServer

import (
	"fmt"
	"strings"
	"testing"
)

func TestRenderLeaderboard(t *testing.T) {
	board := [][]string{
		{"alice", "100.5"},
		{"bob", "75.2"},
		{"carol", "50.0"},
	}
	got := renderLeaderboard("monthly miles", board)

	if !strings.Contains(got, `<div class="lb-title">Monthly Miles</div>`) {
		t.Fatalf("expected title-cased header, got %q", got)
	}
	for _, name := range []string{"alice", "bob", "carol"} {
		if !strings.Contains(got, `<span class="lb-user">(`+name+`)</span>`) {
			t.Fatalf("expected user span for %q, got %q", name, got)
		}
	}
	if !strings.Contains(got, `<span class="lb-score">100.5</span><span class="lb-user">(alice)</span>`) {
		t.Fatalf("expected adjacent score+user spans for alice, got %q", got)
	}
}

func TestRenderLeaderboardTruncatesToMax(t *testing.T) {
	var board [][]string
	for i := 1; i <= maxLeaderboardRows+2; i++ {
		board = append(board, []string{fmt.Sprintf("u%d", i), fmt.Sprint(100 - i)})
	}
	got := renderLeaderboard("top", board)

	// "u1" is a substring of "u10", so match the rendered span, not the name.
	for i := 1; i <= maxLeaderboardRows+2; i++ {
		span := fmt.Sprintf(`<span class="lb-user">(u%d)</span>`, i)
		if want := i <= maxLeaderboardRows; strings.Contains(got, span) != want {
			t.Fatalf("u%d present = %v, want %v: %q", i, !want, want, got)
		}
	}
}

func TestRenderLeaderboardSmallerThanMax(t *testing.T) {
	board := [][]string{
		{"alice", "100"},
		{"bob", "50"},
	}
	got := renderLeaderboard("tiny", board)
	if !strings.Contains(got, "alice") || !strings.Contains(got, "bob") {
		t.Fatalf("expected both names, got %q", got)
	}
}

func TestRenderLeaderboardEmpty(t *testing.T) {
	got := renderLeaderboard("nobody", nil)
	want := `<div class="lb-grid"><div class="lb-title">Nobody</div></div>`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Scores render in their own span (no space-padding) so the CSS grid can
// auto-size the column.
func TestRenderLeaderboardNoSpacePadding(t *testing.T) {
	board := [][]string{
		{"alice", "123"},
		{"bob", "15"},
		{"carol", "7"},
	}
	got := renderLeaderboard("guesses", board)

	wantSpans := []string{
		`<span class="lb-score">123</span><span class="lb-user">(alice)</span>`,
		`<span class="lb-score">15</span><span class="lb-user">(bob)</span>`,
		`<span class="lb-score">7</span><span class="lb-user">(carol)</span>`,
	}
	for _, want := range wantSpans {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in output, got %q", want, got)
		}
	}
	// No padding spaces between score and user spans.
	if strings.Contains(got, "</span> <span") {
		t.Fatalf("did not expect padding space between score/user spans, got %q", got)
	}
}

// Defensive: usernames are normally [a-zA-Z0-9_] from Twitch, but the
// renderer escapes anything that would break out of the surrounding HTML.
func TestRenderLeaderboardEscapesHTML(t *testing.T) {
	board := [][]string{
		{"<script>", "1"},
	}
	got := renderLeaderboard("xss", board)
	if strings.Contains(got, "<script>") {
		t.Fatalf("expected HTML-escaped username, got %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Fatalf("expected &lt;script&gt; escape, got %q", got)
	}
}
