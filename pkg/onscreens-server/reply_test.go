package onscreensServer

import (
	"strings"
	"testing"
	"testing/synctest"
	"time"

	oe "github.com/adanalife/tripbot/pkg/onscreens-events"
	"github.com/nats-io/nats.go"
)

func replyMsg(username, msg string) *nats.Msg {
	return &nats.Msg{
		Subject: "tripbot.test.onscreens.reply.show.tiktok",
		Data:    []byte(`{"username":"` + username + `","msg":"` + msg + `"}`),
	}
}

// A burst shows one reply at a time, in arrival order, each for the full hold
// with a blank gap after it — and the overlay ends hidden.
func TestReplyQueueShowsABurstOneAtATime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := New(Config{Version: "test", Conf: testConf})
		defer srv.Shutdown(t.Context())

		srv.handleReplyShow(replyMsg("ann", "first"))
		srv.handleReplyShow(replyMsg("bob", "second"))
		synctest.Wait()

		if !srv.Reply.IsShowing() || !strings.Contains(srv.Reply.Content(), "first") {
			t.Fatalf("at 0s: showing=%v content=%q, want ann's reply up", srv.Reply.IsShowing(), srv.Reply.Content())
		}
		time.Sleep(replyHold - time.Millisecond)
		synctest.Wait()
		if !srv.Reply.IsShowing() || !strings.Contains(srv.Reply.Content(), "first") {
			t.Fatalf("just before the hold ends: ann's reply should still be up, got %q", srv.Reply.Content())
		}
		time.Sleep(2 * time.Millisecond)
		synctest.Wait()
		if srv.Reply.IsShowing() {
			t.Fatal("in the gap: overlay should be hidden between replies")
		}
		time.Sleep(replyGap)
		synctest.Wait()
		if !srv.Reply.IsShowing() || !strings.Contains(srv.Reply.Content(), "second") {
			t.Fatalf("after the gap: showing=%v content=%q, want bob's reply up", srv.Reply.IsShowing(), srv.Reply.Content())
		}
		time.Sleep(replyHold + replyGap)
		synctest.Wait()
		if srv.Reply.IsShowing() {
			t.Fatal("queue drained: overlay should be hidden")
		}
	})
}

// One chatter holds one place in line: repeating a command replaces their
// waiting answer rather than queueing another, and a full queue drops newcomers.
func TestReplyQueueOnePlacePerChatterAndBounded(t *testing.T) {
	q := newReplyQueue()
	if !q.push(oe.ReplyShow{Username: "ann", Msg: "old"}) || !q.push(oe.ReplyShow{Username: "ann", Msg: "new"}) {
		t.Fatal("a repeat from the same chatter should be kept")
	}
	if len(q.pending) != 1 || q.pending[0].Msg != "new" {
		t.Fatalf("pending = %+v, want ann's latest answer only", q.pending)
	}
	for _, who := range []string{"b", "c", "d", "e"} {
		if !q.push(oe.ReplyShow{Username: who, Msg: "x"}) {
			t.Fatalf("push %s refused below the cap", who)
		}
	}
	if q.push(oe.ReplyShow{Username: "late", Msg: "x"}) {
		t.Fatal("push past replyQueueMax should be dropped")
	}
	if !q.push(oe.ReplyShow{Username: "c", Msg: "again"}) {
		t.Fatal("a queued chatter can still replace their answer when the queue is full")
	}
}

// Malformed and incomplete payloads never reach the queue.
func TestHandleReplyShowRejectsIncomplete(t *testing.T) {
	s := &Server{cfg: testConf, replies: newReplyQueue()}
	for _, data := range []string{`not json`, `{"username":"ann"}`, `{"msg":"hi"}`} {
		s.handleReplyShow(&nats.Msg{Subject: "s", Data: []byte(data)})
	}
	if len(s.replies.pending) != 0 {
		t.Fatalf("pending = %+v, want none", s.replies.pending)
	}
}

// Both strings come from chat and the browser source injects the card as HTML,
// so neither may carry markup through.
func TestRenderReplyEscapesChatText(t *testing.T) {
	got := renderReply(`<img src=x>`, "try `!help` <script>")
	if strings.Contains(got, "<img") || strings.Contains(got, "<script") {
		t.Fatalf("unescaped markup in %q", got)
	}
	if !strings.Contains(got, "@&lt;img src=x&gt;") || !strings.Contains(got, "<code>!help</code>") {
		t.Fatalf("got %q, want the escaped name and a rendered code span", got)
	}
}
