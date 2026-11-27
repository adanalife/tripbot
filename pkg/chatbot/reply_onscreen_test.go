package chatbot

import (
	"context"
	"reflect"
	"testing"

	"github.com/adanalife/tripbot/pkg/video"
)

// On a platform with no chat to post to, a command's answer goes to the reply
// overlay credited to its asker, and still reaches Chat for the console mirror.
// Anything said outside a dispatch (the timed chatter) stays off the screen, as
// does every answer on a platform that can post.
func TestReplyGoesOnScreenOnlyWhereChatCannotPost(t *testing.T) {
	asked := withAsker(context.Background(), "Viewer1")
	tests := []struct {
		name            string
		ctx             context.Context
		repliesOnScreen bool
		msg             string
		want            []string
	}{
		{"read-only platform, command answer", asked, true, "It's 72°F", []string{`ShowReply("Viewer1", "It's 72°F")`}},
		{"action prefix is stripped", asked, true, "/me waves", []string{`ShowReply("Viewer1", "waves")`}},
		{"read-only platform, no asker", context.Background(), true, "timed chatter", nil},
		{"platform that can post", asked, false, "It's 72°F", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestApp(video.Video{})
			overlay := &recordingOnscreens{}
			chat := &recordingChat{}
			a.Onscreens, a.Chat, a.repliesOnScreen = overlay, chat, tt.repliesOnScreen

			a.Reply(tt.ctx, tt.msg)

			if !reflect.DeepEqual(overlay.Calls, tt.want) {
				t.Errorf("overlay calls = %q, want %q", overlay.Calls, tt.want)
			}
			if !reflect.DeepEqual(chat.Says, []string{tt.msg}) {
				t.Errorf("chat says = %q, want the message untouched", chat.Says)
			}
		})
	}
}

// HandleMessage is what names the asker, so a real inbound command carries it
// through to the overlay.
func TestHandleMessageCreditsTheAskerOnScreen(t *testing.T) {
	a := newTestApp(video.Video{})
	overlay := &recordingOnscreens{}
	a.Onscreens, a.repliesOnScreen = overlay, true

	a.HandleMessage(context.Background(), IncomingMessage{User: "TikTokViewer", Text: "!version"})

	if len(overlay.Calls) != 1 || overlay.Calls[0][:len(`ShowReply("TikTokViewer"`)] != `ShowReply("TikTokViewer"` {
		t.Fatalf("overlay calls = %q, want one ShowReply crediting TikTokViewer", overlay.Calls)
	}
}
