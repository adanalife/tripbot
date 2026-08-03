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
)

// guessrGameURL is where a viewer goes to play. Separate from guessrBoardURL
// because that one is redirected at an httptest server in tests; this is copy.
const guessrGameURL = "https://guessr.dana.lol"

// guessrBoardURL is the boards endpoint the guessing game serves at
// guessr.dana.lol. A var, not a const, so tests can point it at an httptest
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
var guessrBoardURL = "https://guessr.dana.lol/api/leaderboard"

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
	ctx, cancel := context.WithTimeout(ctx, guessrTimeout)
	defer cancel()

	u := guessrBoardURL + "?board=" + url.QueryEscape(board)
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
		a.Chat.Say("No " + board + " guessr scores to show right now — play at " + guessrGameURL)
		return
	}

	a.Onscreens.ShowLeaderboard(ctx, title, overlayRows(rows, size))

	msg := title + ": "
	for i, row := range rows {
		msg += fmt.Sprintf("%d. %s (%s)", i+1, row[0], row[1])
		if i+1 != len(rows) {
			msg += ", "
		}
	}
	a.Chat.Say(msg)
}
