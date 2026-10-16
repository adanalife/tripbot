package chatbot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/feature"
	"github.com/adanalife/tripbot/pkg/users"
	"github.com/adanalife/tripbot/pkg/video"
)

// guessrGameURL is where a viewer goes to play. Separate from guessrAPI
// because that one is redirected at an httptest server in tests; this is copy.
const guessrGameURL = "https://guessr.dana.lol"

// guessrAPI is the API the guessing game serves at guessr.dana.lol: the boards
// at /leaderboard and each board row's plays at /guesses. A var, not a const, so tests can point it at an httptest
// server; nothing outside this package can reach it.
//
// The direction is deliberate: the game keeps its scores in the D1 next to its
// own deploy and the bot reads outbound, because this cluster has no inbound
// path and a leaderboard is not a reason to open one.
//
// Always production, including from a staging bot. The board is a public read
// of the same scores either way, and a staging game with nobody playing it
// renders an empty overlay — which is worse for checking the render than the
// real thing.
var guessrAPI = "https://guessr.dana.lol/api"

// guessrTimeout bounds the fetch so a hung Cloudflare response can't stall the
// rotation tick. The overlay job runs every five minutes; anything slower than
// this is a board nobody is waiting for.
const guessrTimeout = 5 * time.Second

// guessrBoard fetches one of the game's boards: the rows in the shape
// ShowLeaderboard wants, and the span they cover. The span is worth carrying
// because the daily board is the last *closed* date rather than today — the
// game leaves a date open until midnight in the last timezone to reach it — so
// a title that didn't name the day would be quietly wrong for the first hours
// of a stream.
func guessrBoard(ctx context.Context, board string) (string, [][]string, error) {
	return guessrBoardFor(ctx, board, "")
}

// guessrBoardFor is guessrBoard pinned to a span: month is a YYYY-MM the game
// serves as a finished month's final standings (empty asks for the running
// span, which is what guessrBoard does). The game refuses a month that has not
// started, so a caller passing one gets the error rather than an empty board.
func guessrBoardFor(ctx context.Context, board, month string) (string, [][]string, error) {
	ctx, cancel := context.WithTimeout(ctx, guessrTimeout)
	defer cancel()

	q := url.Values{"board": {board}}
	if month != "" {
		q.Set("month", month)
	}
	u := guessrAPI + "/leaderboard?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("guessr %s board: status %d", board, resp.StatusCode)
	}

	// Rows arrive as [name, points] pairs — a string next to a number, so the
	// element type has to be any. UseNumber keeps the score as the digits that
	// were sent rather than routing it through float64, where a large enough
	// value would render in scientific notation on the overlay.
	var body struct {
		Period string  `json:"period"`
		Rows   [][]any `json:"rows"`
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		return "", nil, err
	}

	rows := make([][]string, 0, len(body.Rows))
	for _, row := range body.Rows {
		// Short rows are dropped rather than padded: every consumer indexes
		// [0] and [1] positionally, and the overlay renderer that does so runs
		// in another process where the panic would take the whole thing down.
		if len(row) < 2 {
			continue
		}
		cells := make([]string, 0, len(row))
		for _, cell := range row {
			cells = append(cells, fmt.Sprint(cell))
		}
		rows = append(rows, cells)
	}
	return body.Period, rows, nil
}

// withMisses returns a copy of an overlay board with each player's latest
// miss after their name — "Patient Delta · 23 km NW", where the pin landed
// relative to the truth. A row whose plays can't be fetched, don't match it, or
// have no closed round yet is left exactly as it was: the line is a garnish,
// and the board without it is the one the overlay showed all along.
//
// The board is one player per row but several rounds each, so the line is the
// most recent closed round's — the one the player is likeliest to remember.
//
// ponytail: one request per row, sequential under one shared timeout; a hung
// game costs the lines, not the board. Concurrent fetches if ten rows ever read
// slow on stream.
func withMisses(ctx context.Context, board string, rows [][]string) [][]string {
	ctx, cancel := context.WithTimeout(ctx, guessrTimeout)
	defer cancel()

	out := make([][]string, len(rows))
	for i, row := range rows {
		out[i] = row
		miss, err := guessrMiss(ctx, board, i+1, row[0])
		if err != nil {
			slog.DebugContext(ctx, "no guessr miss line", "err", err, "board", board, "rank", i+1)
			continue
		}
		if miss != "" {
			out[i] = append([]string{row[0] + " · " + miss}, row[1:]...)
		}
	}
	return out
}

// guessrMiss is the miss line for the player at rank on board, or "" when none
// of their rounds has closed. name is the board row's label, checked against
// the plays' owner: ranks are re-resolved per request, so a board that moved
// between the two reads would otherwise caption one player with another's miss.
func guessrMiss(ctx context.Context, board string, rank int, name string) (string, error) {
	q := url.Values{"board": {board}, "rank": {fmt.Sprint(rank)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, guessrAPI+"/guesses?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("guessr %s guesses: status %d", board, resp.StatusCode)
	}

	// The coordinates are null on a round still open, which the game withholds
	// because they would be a public copy of today's answers.
	var body struct {
		Name string `json:"name"`
		Rows []struct {
			Km        float64  `json:"km"`
			GuessLat  *float64 `json:"guess_lat"`
			GuessLng  *float64 `json:"guess_lng"`
			AnswerLat *float64 `json:"answer_lat"`
			AnswerLng *float64 `json:"answer_lng"`
		} `json:"rows"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	// The board numbers colliding names ("Sam (2)"); the plays carry the raw one.
	if body.Name == "" || !strings.HasPrefix(name, body.Name) {
		return "", fmt.Errorf("guessr rank %d is %q, board says %q", rank, body.Name, name)
	}
	for i := len(body.Rows) - 1; i >= 0; i-- {
		r := body.Rows[i]
		if r.GuessLat == nil || r.GuessLng == nil || r.AnswerLat == nil || r.AnswerLng == nil {
			continue
		}
		return missLine(r.Km,
			video.Bearing(video.Moment{Lat: *r.AnswerLat, Lng: *r.AnswerLng}, video.Moment{Lat: *r.GuessLat, Lng: *r.GuessLng})), nil
	}
	return "", nil
}

// missLine renders a miss as "23 km NW". The distance is the game's own km, the
// one it scored, rather than a second haversine that could round differently.
// Under a kilometre the direction is noise, so it reads as a bullseye.
func missLine(km, bearing float64) string {
	if km < 1 {
		return "bullseye"
	}
	return fmt.Sprintf("%.0f km %s", km, video.CompassAbbrev(bearing))
}

// guessrLeaderboardCmd answers !guessr with the game's board, on screen and in
// chat. Daily by default — the topical one — with "monthly" for the running
// total, so both boards the rotation shows are reachable on demand.
func (a *App) guessrLeaderboardCmd(ctx context.Context, user *users.User, params []string) {
	slog.InfoContext(ctx, "ran !guessr", "username", user.Username)

	// The same flag the rotation reads. It exists so the boards can leave the
	// overlay from the console without a deploy, and a command that ignored it
	// would put one straight back.
	if !a.Flags.Bool(ctx, guessrBoardFlagKey, feature.EvalContext{
		Username: user.Username,
		Channel:  a.Cfg.ChannelName,
		Env:      a.Cfg.Environment,
	}) {
		slog.InfoContext(ctx, "guessr leaderboard disabled by feature flag", "flag", guessrBoardFlagKey)
		return
	}

	board, parse, display, size := "daily", "2006-01-02", "January 2", onscreenRows
	if len(params) > 0 && strings.HasPrefix(strings.ToLower(params[0]), "month") {
		board, parse, display, size = "monthly", "2006-01", "January", guessrMonthlyRows
	}

	title, rows := a.guessrLeaderboard(ctx, board, parse, display)
	// No rows covers both a board nobody has played and a game this cluster
	// can't reach, so the reply has to be true of either.
	if len(rows) == 0 {
		a.Reply(ctx, "No "+board+" guessr scores to show right now — play at "+guessrGameURL)
		return
	}

	a.Onscreens.ShowLeaderboard(ctx, title, withMisses(ctx, board, overlayRows(rows, size)))

	a.Reply(ctx, title+": "+rankedList(rows, ""))
}
