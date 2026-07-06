package chatbot

import (
	"context"
	"testing"

	"github.com/adanalife/tripbot/pkg/video"
)

func TestHandleYouTubeMessage_RunsCommandWithoutLogin(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec
	// UserSessions stays nil: if HandleYouTubeMessage attempted the
	// LoginIfNecessary step the Twitch path does, this test would panic —
	// passing proves the skip-login contract.

	app.HandleYouTubeMessage(context.Background(), IncomingMessage{User: "YouTubeViewer", Text: "!help"})

	if len(rec.Says) == 0 {
		t.Fatal("expected !help to dispatch and reply via App.Chat")
	}
}

func TestHandleYouTubeMessage_NonCommandIsQuiet(t *testing.T) {
	app := newTestApp(video.Video{})
	rec := &recordingChat{}
	app.Chat = rec

	app.HandleYouTubeMessage(context.Background(), IncomingMessage{User: "YouTubeViewer", Text: "nice stream"})

	if len(rec.Says) != 0 {
		t.Fatalf("plain chatter should not reply; got %v", rec.Says)
	}
}
