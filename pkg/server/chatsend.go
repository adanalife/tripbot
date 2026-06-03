package server

import (
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	chatEvents "github.com/adanalife/tripbot/pkg/chat-events"
	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	mytwitch "github.com/adanalife/tripbot/pkg/twitch"
)

// maxChatMessageLen is Twitch's per-message character limit; the form's
// maxlength mirrors it client-side and this is the server-side guard.
const maxChatMessageLen = 500

// chatSendHandler publishes an operator "send a chat message" command onto
// NATS (tripbot.<env>.chat.send) for cmd/tripbot's subscriber to send as the
// chosen identity. Fire-and-forget: the panel renders the line optimistically
// and reconciles it when the message round-trips back through the chat.message
// SSE stream, so this just validates + publishes and returns 204.
//
// No app-layer auth gate (tailnet-only Ingress, like the other /admin POSTs).
// Identity is validated against the known values; whether that identity is
// actually logged in is the send path's concern — a logged-out broadcaster send
// fails at the subscriber and the optimistic line times out red.
func (s *Server) chatSendHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	identity := r.FormValue("identity")
	if identity != chatEvents.IdentityBot && identity != chatEvents.IdentityBroadcaster {
		http.Error(w, "identity must be 'bot' or 'broadcaster'", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(r.FormValue("text"))
	if text == "" {
		http.Error(w, "text required", http.StatusBadRequest)
		return
	}
	if len(text) > maxChatMessageLen {
		http.Error(w, "text too long", http.StatusRequestEntityTooLarge)
		return
	}

	payload, err := json.Marshal(chatEvents.Send{
		Envelope: chatEvents.NewEnvelope(),
		Identity: identity,
		Text:     text,
	})
	if err != nil {
		slog.ErrorContext(r.Context(), "chat.send: marshal", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.publisher.Publish(r.Context(), chatEvents.SendSubject(c.Conf.Environment), payload)
	w.WriteHeader(http.StatusNoContent)
}

// sendIdentity is one selectable "send as" option in the chat form.
type sendIdentity struct {
	Account  string // "bot" | "broadcaster" — the chatEvents identity value
	Username string // the Twitch login, shown on the toggle + used by the JS optimistic line
}

// availableSendIdentities narrows the per-identity token statuses to the ones
// that can actually send right now — healthy (Reason == "") means logged in.
// A logged-out identity is omitted so the form never offers a send that would
// just fail. Order follows TokenStatuses (bot first, broadcaster second).
func availableSendIdentities(statuses []mytwitch.AccountTokenStatus) []sendIdentity {
	var out []sendIdentity
	for _, st := range statuses {
		if st.Reason == "" {
			out = append(out, sendIdentity{Account: st.Account, Username: st.LoginAs})
		}
	}
	return out
}

type sendFormData struct {
	Identities []sendIdentity // available (logged-in) identities; drives the toggle
	Multi      bool           // >1 identity → show the radio toggle; ==1 → a hidden input
	BotUser    string         // bot login when available, else "" — JS maps identity→username
	BcastUser  string         // broadcaster login when available, else ""
}

// sendFormTmpl renders the chat send form, or a muted hint when no identity is
// logged in. The data-*-user attributes let the page's JS label the optimistic
// (greyed) line with the right username so it reconciles against the real line
// when it round-trips back on the chat.message stream.
var sendFormTmpl = template.Must(template.New("sendform").Parse(
	`{{if .Identities}}<form class="chat-send" hx-post="/admin/chat/send" hx-swap="none" autocomplete="off" data-bot-user="{{.BotUser}}" data-broadcaster-user="{{.BcastUser}}">` +
		`{{if .Multi}}<div class="chat-send-as">{{range $i, $id := .Identities}}<label><input type="radio" name="identity" value="{{$id.Account}}"{{if eq $i 0}} checked{{end}}> {{$id.Username}}</label>{{end}}</div>` +
		`{{else}}<input type="hidden" name="identity" value="{{(index .Identities 0).Account}}"><span class="chat-send-as-single">as {{(index .Identities 0).Username}}</span>{{end}}` +
		`<div class="chat-send-row"><input class="chat-send-text" type="text" name="text" maxlength="500" placeholder="send a message…" required><button type="submit">send</button></div>` +
		`</form>` +
		`{{else}}<p class="chat-send-empty">sign in to send messages</p>{{end}}`))

func renderSendForm(statuses []mytwitch.AccountTokenStatus) string {
	ids := availableSendIdentities(statuses)
	data := sendFormData{Identities: ids, Multi: len(ids) > 1}
	for _, id := range ids {
		switch id.Account {
		case chatEvents.IdentityBot:
			data.BotUser = id.Username
		case chatEvents.IdentityBroadcaster:
			data.BcastUser = id.Username
		}
	}
	var sb strings.Builder
	if err := sendFormTmpl.Execute(&sb, data); err != nil {
		slog.Error("admin: render send form", "err", err)
		return ""
	}
	return sb.String()
}
