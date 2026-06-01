package users

import (
	"context"
	"strings"
	"testing"
)

func TestStrToFloat32Valid(t *testing.T) {
	tests := []struct {
		in   string
		want float32
	}{
		{"0", 0},
		{"1.5", 1.5},
		{"42.0", 42.0},
		{"-3.25", -3.25},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := strToFloat32(context.Background(), tt.in)
			if got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLeaderboardContent(t *testing.T) {
	board := [][]string{
		{"alice", "100.5"},
		{"bob", "75.2"},
		{"carol", "50.0"},
	}
	got := LeaderboardContent("monthly miles", board)

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

func TestLeaderboardContentTruncatesToFive(t *testing.T) {
	board := [][]string{
		{"u1", "10"}, {"u2", "9"}, {"u3", "8"}, {"u4", "7"},
		{"u5", "6"}, {"u6", "5"}, {"u7", "4"},
	}
	got := LeaderboardContent("top", board)

	if strings.Contains(got, "u6") || strings.Contains(got, "u7") {
		t.Fatalf("expected truncation to 5 entries, got %q", got)
	}
	for _, name := range []string{"u1", "u2", "u3", "u4", "u5"} {
		if !strings.Contains(got, name) {
			t.Fatalf("expected %q in top-5, got %q", name, got)
		}
	}
}

func TestLeaderboardContentSmallerThanFive(t *testing.T) {
	board := [][]string{
		{"alice", "100"},
		{"bob", "50"},
	}
	got := LeaderboardContent("tiny", board)
	if !strings.Contains(got, "alice") || !strings.Contains(got, "bob") {
		t.Fatalf("expected both names, got %q", got)
	}
}

func TestLeaderboardContentEmpty(t *testing.T) {
	got := LeaderboardContent("nobody", nil)
	want := `<div class="lb-grid"><div class="lb-title">Nobody</div></div>`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Scores render in their own span (no space-padding) so the CSS grid can
// auto-size the column.
func TestLeaderboardContentNoSpacePadding(t *testing.T) {
	board := [][]string{
		{"alice", "123"},
		{"bob", "15"},
		{"carol", "7"},
	}
	got := LeaderboardContent("guesses", board)

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
func TestLeaderboardContentEscapesHTML(t *testing.T) {
	board := [][]string{
		{"<script>", "1"},
	}
	got := LeaderboardContent("xss", board)
	if strings.Contains(got, "<script>") {
		t.Fatalf("expected HTML-escaped username, got %q", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Fatalf("expected &lt;script&gt; escape, got %q", got)
	}
}

func TestRemoveFromLeaderboard(t *testing.T) {
	s := New(noopChatterSource{})
	s.lifetimeLeaderboard = [][]string{
		{"alice", "100"}, {"bob", "75"}, {"carol", "50"},
	}

	s.removeFromLeaderboard("bob")

	if len(s.lifetimeLeaderboard) != 2 {
		t.Fatalf("expected 2 entries after remove, got %d", len(s.lifetimeLeaderboard))
	}
	for _, pair := range s.lifetimeLeaderboard {
		if pair[0] == "bob" {
			t.Fatal("bob still present after remove")
		}
	}
}

func TestRemoveFromLeaderboardMissing(t *testing.T) {
	s := New(noopChatterSource{})
	s.lifetimeLeaderboard = [][]string{
		{"alice", "100"}, {"bob", "75"},
	}

	s.removeFromLeaderboard("nobody")

	if len(s.lifetimeLeaderboard) != 2 {
		t.Fatalf("expected unchanged length after missing remove, got %d", len(s.lifetimeLeaderboard))
	}
}

func TestRemoveFromLeaderboardEmpty(t *testing.T) {
	s := New(noopChatterSource{})

	s.removeFromLeaderboard("alice")

	if len(s.lifetimeLeaderboard) != 0 {
		t.Fatalf("expected empty leaderboard, got %v", s.lifetimeLeaderboard)
	}
}

// insertIntoLeaderboard pulls miles from User.CurrentMiles. The user isn't in
// the default session, so CurrentMiles returns User.Miles directly (no session
// bonus) — making the math deterministic.
func TestInsertIntoLeaderboardOrdersByMilesDesc(t *testing.T) {
	s := New(noopChatterSource{})
	s.lifetimeLeaderboard = [][]string{
		{"existing-1", "100"},
		{"existing-2", "50"},
		{"existing-3", "10"},
	}

	u := User{Username: "newcomer", Miles: 75}
	s.insertIntoLeaderboard(context.Background(), u)

	if len(s.lifetimeLeaderboard) != 4 {
		t.Fatalf("expected 4 entries after insert, got %d: %v",
			len(s.lifetimeLeaderboard), s.lifetimeLeaderboard)
	}
	if s.lifetimeLeaderboard[1][0] != "newcomer" {
		t.Fatalf("expected newcomer at index 1 (between 100 and 50), got order %v", s.lifetimeLeaderboard)
	}
}

func TestInsertIntoLeaderboardReplacesExistingUser(t *testing.T) {
	s := New(noopChatterSource{})
	s.lifetimeLeaderboard = [][]string{
		{"alice", "100"},
		{"bob", "50"},
	}

	// Bob's miles increased to 200 — should jump above alice and replace his old row.
	u := User{Username: "bob", Miles: 200}
	s.insertIntoLeaderboard(context.Background(), u)

	if len(s.lifetimeLeaderboard) != 2 {
		t.Fatalf("expected length unchanged after replace, got %d: %v",
			len(s.lifetimeLeaderboard), s.lifetimeLeaderboard)
	}
	if s.lifetimeLeaderboard[0][0] != "bob" {
		t.Fatalf("expected bob to be #1 with 200 miles, got %v", s.lifetimeLeaderboard)
	}
}
