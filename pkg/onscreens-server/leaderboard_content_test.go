package onscreensServer

import (
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

func TestRenderLeaderboardTruncatesToFive(t *testing.T) {
	board := [][]string{
		{"u1", "10"}, {"u2", "9"}, {"u3", "8"}, {"u4", "7"},
		{"u5", "6"}, {"u6", "5"}, {"u7", "4"},
	}
	got := renderLeaderboard("top", board)

	if strings.Contains(got, "u6") || strings.Contains(got, "u7") {
		t.Fatalf("expected truncation to 5 entries, got %q", got)
	}
	for _, name := range []string{"u1", "u2", "u3", "u4", "u5"} {
		if !strings.Contains(got, name) {
			t.Fatalf("expected %q in top-5, got %q", name, got)
		}
	}
}

func TestRenderLeaderboardSmallerThanFive(t *testing.T) {
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
