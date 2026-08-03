package chatbot

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/video"
)

func TestPickLeaderboard(t *testing.T) {
	tests := []struct {
		roll float64
		want leaderboardKind
	}{
		{0.0, totalMilesLeaderboard},
		{0.0499, totalMilesLeaderboard},
		{0.05, guessLeaderboard},
		{0.3666, guessLeaderboard},
		{0.3667, monthlyMilesLeaderboard},
		{0.6833, monthlyMilesLeaderboard},
		{0.6834, guessrDailyLeaderboard},
		{0.8416, guessrDailyLeaderboard},
		{0.8417, guessrMonthlyLeaderboard},
		{0.9999, guessrMonthlyLeaderboard},
	}
	for _, tt := range tests {
		if got := pickLeaderboard(tt.roll, true); got != tt.want {
			t.Errorf("pickLeaderboard(%v) = %v, want %v", tt.roll, got, tt.want)
		}
	}
}

// With the flag off the rotation is the three-board split that predates the
// guessing game — the boards are removed from the draw, not drawn and then
// skipped, so no tick is spent on a board that renders nothing.
func TestPickLeaderboard_GuessrDisabled(t *testing.T) {
	tests := []struct {
		roll float64
		want leaderboardKind
	}{
		{0.0, totalMilesLeaderboard},
		{0.0499, totalMilesLeaderboard},
		{0.05, guessLeaderboard},
		{0.5249, guessLeaderboard},
		{0.525, monthlyMilesLeaderboard},
		{0.9999, monthlyMilesLeaderboard},
	}
	for _, tt := range tests {
		if got := pickLeaderboard(tt.roll, false); got != tt.want {
			t.Errorf("pickLeaderboard(%v, false) = %v, want %v", tt.roll, got, tt.want)
		}
	}
	for i := 0; i < 1000; i++ {
		got := pickLeaderboard(float64(i)/1000, false)
		if got == guessrDailyLeaderboard || got == guessrMonthlyLeaderboard {
			t.Fatalf("roll %v picked a guessr board with the flag off", float64(i)/1000)
		}
	}
}

func TestShowRotatingLeaderboard_MonthlyMiles(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec
	app.Scoreboards = &recordingScoreboards{
		Month: "July",
		Miles: [][]string{{"viewer1", "12.5"}, {"viewer2", "3.2"}},
	}

	app.showRotatingLeaderboard(context.Background(), 0.99) // monthly miles

	want := `ShowLeaderboard("July Miles", 2 rows)`
	if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], want) {
		t.Errorf("expected one July Miles overlay call, got %v", rec.Calls)
	}
}

func TestShowRotatingLeaderboard_Guess(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec
	app.Scoreboards = &recordingScoreboards{Guesses: [][]string{{"viewer1", "7"}}}

	app.showRotatingLeaderboard(context.Background(), 0.3) // guess

	if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], `ShowLeaderboard("Correct Guesses This Month", 1 rows)`) {
		t.Errorf("expected one guess overlay call, got %v", rec.Calls)
	}
}

// An empty guess board (early in the month) falls back to monthly miles
// rather than skipping the rotation slot.
func TestShowRotatingLeaderboard_EmptyGuess_FallsBackToMonthlyMiles(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec
	app.Scoreboards = &recordingScoreboards{
		Month: "July",
		Miles: [][]string{{"viewer1", "12.5"}},
		// Guesses stays empty — early in the month, nobody has scored yet.
	}

	app.showRotatingLeaderboard(context.Background(), 0.3) // guess → empty → miles

	want := `ShowLeaderboard("July Miles", 1 rows)`
	if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], want) {
		t.Errorf("expected fallback to July Miles overlay, got %v", rec.Calls)
	}
}

func TestShowRotatingLeaderboard_TotalMiles_TruncatesToSize(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec

	var lifetime [][]string
	for i := 0; i < 12; i++ {
		lifetime = append(lifetime, []string{fmt.Sprintf("viewer%d", i), "100.0"})
	}
	app.Sessions = &recordingSessions{Leaderboard: lifetime}

	app.showRotatingLeaderboard(context.Background(), 0.01) // total miles

	want := `ShowLeaderboard("Total Miles", 5 rows)`
	if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], want) {
		t.Errorf("expected truncated Total Miles overlay (%s), got %v", want, rec.Calls)
	}
}

// If both the pick and the monthly-miles fallback are empty, no overlay is
// published at all.
func TestShowRotatingLeaderboard_AllEmpty_SkipsOverlay(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec
	// total miles via noopSessions returns nil; noopScoreboards makes the
	// monthly-miles fallback empty too

	app.showRotatingLeaderboard(context.Background(), 0.01)

	if len(rec.Calls) != 0 {
		t.Errorf("expected no overlay call when every board is empty, got %v", rec.Calls)
	}
}

// The odds Dana asked for, stated as the property rather than as thresholds:
// each guessing-game board comes up about half as often as a miles or chat-guess
// board. Sampling the roll space beats re-deriving the boundaries, because it
// still holds if the shares are ever expressed some other way.
func TestPickLeaderboard_GuessrBoardsGetHalfShare(t *testing.T) {
	const samples = 100000
	counts := map[leaderboardKind]int{}
	for i := 0; i < samples; i++ {
		counts[pickLeaderboard(float64(i)/samples, true)]++
	}
	for _, guessr := range []leaderboardKind{guessrDailyLeaderboard, guessrMonthlyLeaderboard} {
		for _, full := range []leaderboardKind{guessLeaderboard, monthlyMilesLeaderboard} {
			ratio := float64(counts[full]) / float64(counts[guessr])
			if ratio < 1.95 || ratio > 2.05 {
				t.Errorf("board %v should come up ~2x as often as %v, got %.2fx", full, guessr, ratio)
			}
		}
	}
}

func TestShowRotatingLeaderboard_GuessrDaily(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("board"); got != "daily" {
			t.Errorf("expected the daily board, got %q", got)
		}
		fmt.Fprint(w, `{"board":"daily","period":"2026-08-01","rows":[["Lucky Overpass",4993],["anonymous",120]]}`)
	}))
	defer srv.Close()
	swapGuessrURL(t, srv.URL)

	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec
	app.Flags = &recordingFlags{Set: map[string]bool{guessrBoardFlagKey: true}}

	app.showRotatingLeaderboard(context.Background(), 0.7) // guessr daily

	want := `ShowLeaderboard("August 1 Guessr", 2 rows)`
	if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], want) {
		t.Errorf("expected %s, got %v", want, rec.Calls)
	}
}

func TestShowRotatingLeaderboard_GuessrMonthly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"board":"monthly","period":"2026-08","rows":[["Lucky Overpass",4993]]}`)
	}))
	defer srv.Close()
	swapGuessrURL(t, srv.URL)

	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec
	app.Flags = &recordingFlags{Set: map[string]bool{guessrBoardFlagKey: true}}

	app.showRotatingLeaderboard(context.Background(), 0.9) // guessr monthly

	want := `ShowLeaderboard("August Guessr", 1 rows)`
	if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], want) {
		t.Errorf("expected %s, got %v", want, rec.Calls)
	}
}

// The one board that keeps a full ten rows onscreen. Both guessr boards come
// off the same fetch, so serving one over-long response to both proves the size
// travels with the board rather than the code path.
func TestShowRotatingLeaderboard_GuessrMonthlyKeepsMoreRows(t *testing.T) {
	var rows []string
	for i := 0; i < 12; i++ {
		rows = append(rows, fmt.Sprintf(`["viewer%d",%d]`, i, 100-i))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		period := "2026-08"
		if r.URL.Query().Get("board") == "daily" {
			period = "2026-08-01"
		}
		fmt.Fprintf(w, `{"period":%q,"rows":[%s]}`, period, strings.Join(rows, ","))
	}))
	defer srv.Close()
	swapGuessrURL(t, srv.URL)

	for _, tc := range []struct {
		name string
		roll float64
		want string
	}{
		{"daily", 0.7, `ShowLeaderboard("August 1 Guessr", 5 rows)`},
		{"monthly", 0.9, `ShowLeaderboard("August Guessr", 10 rows)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newTestApp(video.Video{})
			rec := &recordingOnscreens{}
			app.Onscreens = rec
			app.Flags = &recordingFlags{Set: map[string]bool{guessrBoardFlagKey: true}}

			app.showRotatingLeaderboard(context.Background(), tc.roll)

			if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], tc.want) {
				t.Errorf("expected %s, got %v", tc.want, rec.Calls)
			}
		})
	}
}

// The score is rendered as the digits that were sent. A float64 round-trip
// would print a big enough number in scientific notation on the overlay.
func TestGuessrBoard_LargeScoreKeepsItsDigits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"board":"monthly","period":"2026-08","rows":[["Lucky Overpass",12345678]]}`)
	}))
	defer srv.Close()
	swapGuessrURL(t, srv.URL)

	_, rows, err := guessrBoard(context.Background(), "monthly")
	if err != nil {
		t.Fatalf("guessrBoard: %v", err)
	}
	if rows[0][1] != "12345678" {
		t.Errorf("score rendered as %q, want 12345678", rows[0][1])
	}
}

// Cloudflare Pages answers 200 with the game's HTML for a path no Function
// claims, which is what a bot pointed at a deploy predating the boards API
// gets. That must read as an empty board and fall back, not as rows.
func TestShowRotatingLeaderboard_GuessrServesHTML_FallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "<!doctype html>\n<html lang=\"en\">")
	}))
	defer srv.Close()
	swapGuessrURL(t, srv.URL)

	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec
	app.Flags = &recordingFlags{Set: map[string]bool{guessrBoardFlagKey: true}}
	app.Scoreboards = &recordingScoreboards{
		Month: "August",
		Miles: [][]string{{"viewer1", "12.5"}},
	}

	app.showRotatingLeaderboard(context.Background(), 0.7) // guessr daily → unreachable → miles

	want := `ShowLeaderboard("August Miles", 1 rows)`
	if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], want) {
		t.Errorf("expected fallback to August Miles, got %v", rec.Calls)
	}
}

// A board the game is reachable for but has no rows in yet — a date nobody has
// finished — is the same empty-board fallback, without an error to log.
func TestShowRotatingLeaderboard_GuessrEmpty_FallsBack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"board":"daily","period":"2026-08-01","rows":[]}`)
	}))
	defer srv.Close()
	swapGuessrURL(t, srv.URL)

	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec
	app.Flags = &recordingFlags{Set: map[string]bool{guessrBoardFlagKey: true}}
	app.Scoreboards = &recordingScoreboards{
		Month: "August",
		Miles: [][]string{{"viewer1", "12.5"}},
	}

	app.showRotatingLeaderboard(context.Background(), 0.7)

	if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], `ShowLeaderboard("August Miles", 1 rows)`) {
		t.Errorf("expected fallback to August Miles, got %v", rec.Calls)
	}
}

// !guessr defaults to the daily board, in chat and on the overlay. The third
// row is malformed on purpose: rows are indexed positionally by both consumers,
// and the overlay renderer that does so runs in another process.
func TestGuessrLeaderboardCmd_Daily(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("board"); got != "daily" {
			t.Errorf("expected the daily board, got %q", got)
		}
		fmt.Fprint(w, `{"board":"daily","period":"2026-08-01","rows":[["Lucky Overpass",4993],["Cedar Glacier",732],["Truncated"]]}`)
	}))
	defer srv.Close()
	swapGuessrURL(t, srv.URL)

	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec
	app.Flags = &recordingFlags{Set: map[string]bool{guessrBoardFlagKey: true}}
	out := captureSay(t, app)

	app.guessrLeaderboardCmd(context.Background(), newTestUser("viewer1"), nil)

	want := "August 1 Guessr: 1. Lucky Overpass (4993), 2. Cedar Glacier (732)"
	if got := out(); got != want {
		t.Errorf("chat said %q, want %q", got, want)
	}
	if len(rec.Calls) != 1 || !strings.Contains(rec.Calls[0], `ShowLeaderboard("August 1 Guessr", 2 rows)`) {
		t.Errorf("expected one August 1 Guessr overlay call, got %v", rec.Calls)
	}
}

func TestGuessrLeaderboardCmd_MonthlyParam(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("board"); got != "monthly" {
			t.Errorf("expected the monthly board, got %q", got)
		}
		fmt.Fprint(w, `{"board":"monthly","period":"2026-08","rows":[["Endless Roadside",17163]]}`)
	}))
	defer srv.Close()
	swapGuessrURL(t, srv.URL)

	app := newTestApp(video.Video{})
	app.Flags = &recordingFlags{Set: map[string]bool{guessrBoardFlagKey: true}}
	out := captureSay(t, app)

	app.guessrLeaderboardCmd(context.Background(), newTestUser("viewer1"), []string{"Monthly"})

	if want := "August Guessr: 1. Endless Roadside (17163)"; out() != want {
		t.Errorf("chat said %q, want %q", out(), want)
	}
}

// No rows is both "nobody has played it" and "the game is unreachable" — the
// bot can't tell them apart and neither can the reply, so it points at the game
// instead of claiming either.
func TestGuessrLeaderboardCmd_NoRows_PointsAtTheGame(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"board":"daily","period":"2026-08-01","rows":[]}`)
	}))
	defer srv.Close()
	swapGuessrURL(t, srv.URL)

	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec
	app.Flags = &recordingFlags{Set: map[string]bool{guessrBoardFlagKey: true}}
	out := captureSay(t, app)

	app.guessrLeaderboardCmd(context.Background(), newTestUser("viewer1"), nil)

	if !strings.Contains(out(), guessrGameURL) {
		t.Errorf("expected the game link in %q", out())
	}
	if len(rec.Calls) != 0 {
		t.Errorf("expected no overlay call for an empty board, got %v", rec.Calls)
	}
}

// The flag takes the boards off the overlay; a command that fetched anyway
// would put one back on a live broadcast.
func TestGuessrLeaderboardCmd_FlagOff_NeverFetches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("fetched the guessr board with the flag off")
	}))
	defer srv.Close()
	swapGuessrURL(t, srv.URL)

	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec
	app.Flags = &recordingFlags{} // every key false
	out := captureSay(t, app)

	app.guessrLeaderboardCmd(context.Background(), newTestUser("viewer1"), nil)

	if out() != "" {
		t.Errorf("said %q with the flag off, want silence", out())
	}
	if len(rec.Calls) != 0 {
		t.Errorf("expected no overlay call with the flag off, got %v", rec.Calls)
	}
}

func swapGuessrURL(t *testing.T, url string) {
	t.Helper()
	original := guessrBoardURL
	guessrBoardURL = url
	t.Cleanup(func() { guessrBoardURL = original })
}

// With the flag off, no roll reaches the game at all. Asserted against a server
// that fails the test if it is called, because the point of the flag is taking
// an outbound dependency off a live broadcast — a version that still fetched
// and discarded the rows would pass a "no guessr board on screen" check while
// leaving the call in place.
func TestShowRotatingLeaderboard_FlagOff_NeverFetches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("fetched the guessr board with the flag off")
	}))
	defer srv.Close()
	swapGuessrURL(t, srv.URL)

	app := newTestApp(video.Video{})
	rec := &recordingOnscreens{}
	app.Onscreens = rec
	app.Flags = &recordingFlags{} // every key false
	app.Scoreboards = &recordingScoreboards{
		Month: "August",
		Miles: [][]string{{"viewer1", "12.5"}},
	}

	// The rolls that would land on the guessr boards with the flag on.
	for _, roll := range []float64{0.7, 0.9} {
		app.showRotatingLeaderboard(context.Background(), roll)
	}
	for _, call := range rec.Calls {
		if strings.Contains(call, "Guessr") {
			t.Errorf("a guessr board reached the overlay with the flag off: %s", call)
		}
	}
}
