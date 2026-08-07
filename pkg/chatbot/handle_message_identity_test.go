package chatbot

import (
	"context"
	"testing"

	"github.com/adanalife/tripbot/pkg/video"
)

// gatewayPlatformApp is a test App on a platform that punts identity, so an
// inbound message must reach the command path without a login. UserSessions
// stays nil on purpose: the login step would dereference it, so a panic is what
// a regression looks like here.
func gatewayPlatformApp(t *testing.T) (*App, *recordingChat) {
	t.Helper()
	app := newTestApp(video.Video{})
	app.Platform = platformYouTube
	app.indexCommands() // re-index: the platform decides which commands dispatch
	rec := &recordingChat{}
	app.Chat = rec
	return app, rec
}

func TestHandleMessage_GatewayPlatformRunsCommandWithoutLogin(t *testing.T) {
	app, rec := gatewayPlatformApp(t)

	app.HandleMessage(context.Background(), IncomingMessage{User: "YouTubeViewer", Text: "!help"})

	if len(rec.Says) == 0 {
		t.Fatal("expected !help to dispatch and reply via App.Chat")
	}
}

func TestHandleMessage_GatewayPlatformNonCommandIsQuiet(t *testing.T) {
	app, rec := gatewayPlatformApp(t)

	app.HandleMessage(context.Background(), IncomingMessage{User: "YouTubeViewer", Text: "nice stream"})

	if len(rec.Says) != 0 {
		t.Fatalf("plain chatter should not reply; got %v", rec.Says)
	}
}

// Twitch runs the identity commands (!miles, !leaderboard) and the
// follower/subscriber access checks, all of which read the persisted user — so
// its inbound messages must log the sender in. Routing Twitch to the transient
// path instead is silent: commands still answer, just with an empty user, so
// !miles reports 0 rather than erroring.
//
// The assertion is a panic because UserSessions is a concrete *users.Sessions
// rather than an interface (the App.DB/UserSessions retirement item), so a nil
// one is the only way to observe the login step without a database.
func TestHandleMessage_TwitchLogsTheSenderIn(t *testing.T) {
	app := newTestApp(video.Video{})
	app.Platform = platformTwitch
	app.Chat = &recordingChat{}

	defer func() {
		if recover() == nil {
			t.Error("Twitch inbound did not reach the login step; the sender was left transient")
		}
	}()

	app.HandleMessage(context.Background(), IncomingMessage{User: "TwitchViewer", Text: "nice stream"})
}
