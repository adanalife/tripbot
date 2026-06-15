package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"

	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
)

// chatters is the data seam the chatters endpoint reads through. It returns the
// set of logins currently in chat, refreshed ~60s by the UpdateSession cron via
// the Helix chat/chatters call (moderator:read:chatters). Overridable in tests
// so the handler renders without a live Twitch connection. In-memory read — no
// network call at request time.
var chatters = mytwitch.Chatters

// chattersResponse is the JSON payload the standalone tripbot-console reads to
// render its currently-active-chatters list. The console holds no Twitch/Helix
// access, so it proxies here over the in-namespace Service.
type chattersResponse struct {
	// Chatters is the sorted list of logins currently in chat.
	Chatters []string `json:"chatters"`
	// Count is len(Chatters) — the number of logins in this response. It can be
	// lower than Twitch's reported total when the channel has more than one page
	// of chatters (the cron only caches the first page).
	Count int `json:"count"`
}

// chattersHandler serves GET /api/chatters: the current chatter logins as JSON,
// for the tripbot-console to render its currently-active-chatters panel.
func chattersHandler(w http.ResponseWriter, r *http.Request) {
	set := chatters()
	logins := make([]string, 0, len(set))
	for login := range set {
		logins = append(logins, login)
	}
	sort.Strings(logins)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(chattersResponse{Chatters: logins, Count: len(logins)}); err != nil {
		slog.ErrorContext(r.Context(), "couldn't encode chatters", "err", err)
	}
}
