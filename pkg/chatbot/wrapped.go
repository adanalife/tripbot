package chatbot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/adanalife/tripbot/pkg/users"
)

// wrappedCmd implements !wrapped: a viewer's month of the guessing game in one
// chat line, the stream-side twin of the app's Guessr Wrapped. "!wrapped last"
// reads the previous month, which is what a viewer wants on the 1st.
func (a *App) wrappedCmd(ctx context.Context, user *users.User, params []string) {
	slog.InfoContext(ctx, "ran !wrapped", "username", user.Username)

	at := time.Now()
	if len(params) > 0 && strings.EqualFold(params[0], "last") {
		at = a.Scoreboards.LastMonth()
	}
	w, err := a.Scoreboards.Wrapped(ctx, user.Username, at)
	if err != nil {
		slog.ErrorContext(ctx, "wrapped read failed", "err", err, "username", user.Username)
		a.Reply(ctx, "Can't add that up right now, sorry!")
		return
	}
	a.Reply(ctx, wrappedLine(user.Username, w))
}

// wrappedLine renders a Wrapped as the one chat line !wrapped replies with.
// Each clause drops out when its number is zero, so a quiet month reads as a
// short sentence rather than a row of zeros. States are only ever named in
// the past tense — the one the viewer kept missing — never the clip on screen.
func wrappedLine(username string, w scoreboards.Wrapped) string {
	month := w.Month.Format("January")
	if w.Guesses == 0 && w.Minutes < 1 {
		return fmt.Sprintf("@%s, nothing from you in %s yet — !guess a state and come back!", username, month)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "@%s, your %s: ", username, month)
	if w.Guesses == 0 {
		b.WriteString("no guesses")
	} else {
		fmt.Fprintf(&b, "%s, %d right", countNoun(w.Guesses, "guess", "guesses"), w.Correct)
		if w.Rank > 0 {
			fmt.Fprintf(&b, " (#%d of %d on the board)", w.Rank, w.Players)
		}
		if w.States > 1 {
			fmt.Fprintf(&b, ", %d different states called", w.States)
		}
		if w.MissedMost != "" {
			fmt.Fprintf(&b, "; %s gets you most", w.MissedMost)
			if w.CalledIt != "" && !strings.EqualFold(w.CalledIt, w.MissedMost) {
				fmt.Fprintf(&b, " — you keep saying %s", w.CalledIt)
			}
		}
	}
	if w.Minutes >= 1 {
		fmt.Fprintf(&b, ". %s in the van with me 🚐", watchTime(w.Minutes))
	}
	return b.String()
}

// countNoun is "1 guess" / "7 guesses".
func countNoun(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// watchTime renders minutes as "45m", "3h" or "3h 20m".
func watchTime(minutes float64) string {
	h, m := int(minutes)/60, int(minutes)%60
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}
