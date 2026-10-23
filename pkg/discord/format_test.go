package discord

import (
	"strings"
	"testing"
)

func TestLeaderboardEmbed(t *testing.T) {
	cases := []struct {
		name        string
		title       string
		entries     [][]string
		wantNil     bool
		wantTitle   string
		wantContain []string
	}{
		{
			name:    "empty entries returns nil",
			title:   "Anything",
			entries: nil,
			wantNil: true,
		},
		{
			name:        "single entry renders 1.",
			title:       "Monthly Miles",
			entries:     [][]string{{"foo", "12.3"}},
			wantTitle:   "Monthly Miles",
			wantContain: []string{"**1.** foo — 12.3"},
		},
		{
			name:      "three entries render in order",
			title:     "Total Miles",
			entries:   [][]string{{"a", "100.0"}, {"b", "50.0"}, {"c", "25.0"}},
			wantTitle: "Total Miles",
			wantContain: []string{
				"**1.** a — 100.0",
				"**2.** b — 50.0",
				"**3.** c — 25.0",
			},
		},
		{
			// Same competition ranking the chat commands use, so a board
			// posted in both places reads the same in both.
			name:      "ties share the better place",
			title:     "Total Miles",
			entries:   [][]string{{"a", "100.0"}, {"b", "100.0"}, {"c", "25.0"}},
			wantTitle: "Total Miles",
			wantContain: []string{
				"**1.** a — 100.0",
				"**1.** b — 100.0",
				"**3.** c — 25.0",
			},
		},
		{
			name:        "malformed pair skipped",
			title:       "X",
			entries:     [][]string{{"only-one"}, {"a", "1.0"}},
			wantTitle:   "X",
			wantContain: []string{"**1.** a — 1.0"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := leaderboardEmbed(tc.title, tc.entries)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("want nil embed, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("got nil embed, want non-nil")
			}
			if got.Title != tc.wantTitle {
				t.Errorf("title: want %q, got %q", tc.wantTitle, got.Title)
			}
			for _, want := range tc.wantContain {
				if !strings.Contains(got.Description, want) {
					t.Errorf("description missing %q\n--- description ---\n%s", want, got.Description)
				}
			}
		})
	}
}
