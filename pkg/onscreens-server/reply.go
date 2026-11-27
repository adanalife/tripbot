package onscreensServer

import (
	"encoding/json"
	"html"
	"log/slog"
	"sync"
	"time"

	oe "github.com/adanalife/tripbot/pkg/onscreens-events"
	"github.com/nats-io/nats.go"
)

const (
	// replyHold is how long one reply stays up: long enough to read two lines
	// over moving video.
	replyHold = 8 * time.Second
	// replyGap is the blank between two replies. It spans two polls of the
	// browser source, so the card visibly drops before the next one lands and a
	// viewer can tell a new answer from an edit of the old one.
	replyGap = time.Second
	// replyQueueMax bounds the wait: a reply joining a full queue would land
	// most of a minute after its command, when its asker has stopped looking.
	replyQueueMax = 5
)

// replyQueue feeds the reply overlay one answer at a time. Replies arrive in
// bursts — one chatter hammering a command, or several asking at once — and
// showing each as it lands would flash them past unread, so they wait their
// turn here instead.
type replyQueue struct {
	mu      sync.Mutex
	pending []oe.ReplyShow
	// wake has room for one token: a push while the worker is busy leaves it
	// set, and the worker re-checks pending before it next sleeps.
	wake chan struct{}
	hold time.Duration
	gap  time.Duration
}

func newReplyQueue() *replyQueue {
	return &replyQueue{wake: make(chan struct{}, 1), hold: replyHold, gap: replyGap}
}

// push queues ev and reports whether it was kept. A chatter holds one place in
// line: a second answer for the same chatter replaces the one waiting, so
// repeating a command can't crowd everyone else out. A full queue drops the
// newcomer.
func (q *replyQueue) push(ev oe.ReplyShow) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := range q.pending {
		if q.pending[i].Username == ev.Username {
			q.pending[i] = ev
			return true
		}
	}
	if len(q.pending) >= replyQueueMax {
		return false
	}
	q.pending = append(q.pending, ev)
	select {
	case q.wake <- struct{}{}:
	default:
	}
	return true
}

func (q *replyQueue) pop() (oe.ReplyShow, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.pending) == 0 {
		return oe.ReplyShow{}, false
	}
	ev := q.pending[0]
	q.pending = q.pending[1:]
	return ev, true
}

// run shows each queued reply on osc for hold, then blanks it for gap, until
// done closes.
func (q *replyQueue) run(osc *Onscreen, done <-chan struct{}) {
	sleep := func(d time.Duration) bool {
		select {
		case <-done:
			return false
		case <-time.After(d):
			return true
		}
	}
	for {
		ev, ok := q.pop()
		if !ok {
			select {
			case <-done:
				return
			case <-q.wake:
			}
			continue
		}
		osc.Show(renderReply(ev.Username, ev.Msg))
		if !sleep(q.hold) {
			return
		}
		osc.Hide()
		if !sleep(q.gap) {
			return
		}
	}
}

// renderReply builds the reply card's HTML: who is being answered, then the
// answer with inline markdown applied. Both strings come from chat, so the
// name is escaped here and the message inside renderInlineMarkdown.
func renderReply(username, msg string) string {
	return `<div class="reply-card"><span class="reply-who">@` + html.EscapeString(username) +
		`</span><span class="reply-msg">` + renderInlineMarkdown(msg) + `</span></div>`
}

// handleReplyShow queues a command's answer for the reply overlay. Strict like
// middle.show: an answer with no text or no asker is a publisher bug.
func (s *Server) handleReplyShow(m *nats.Msg) {
	var ev oe.ReplyShow
	if err := json.Unmarshal(m.Data, &ev); err != nil {
		slog.Error("nats: decode reply.show", "err", err, "subject", m.Subject)
		return
	}
	if ev.Msg == "" || ev.Username == "" {
		slog.Warn("nats: reply.show missing username or msg", "subject", m.Subject)
		return
	}
	if !s.replies.push(ev) {
		slog.Warn("reply overlay queue full; dropped", "username", ev.Username)
	}
}
