package chatbot

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/video"
)

// gatewayPlatformApp is a test App on a platform that punts identity, so an
// inbound message must reach the command path without a login. Sessions is a
// recorder so the tests can assert the login step was skipped.
func gatewayPlatformApp(t *testing.T) (*App, *recordingChat, *recordingSessions) {
	t.Helper()
	app := newTestApp(video.Video{})
	app.Platform = platformYouTube
	ses := &recordingSessions{}
	app.Sessions = ses
	app.indexCommands() // re-index: the platform decides which commands dispatch
	rec := &recordingChat{}
	app.Chat = rec
	return app, rec, ses
}

func TestHandleMessage_GatewayPlatformRunsCommandWithoutLogin(t *testing.T) {
	app, rec, ses := gatewayPlatformApp(t)

	app.HandleMessage(context.Background(), IncomingMessage{User: "YouTubeViewer", Text: "!help"})

	if len(rec.Says) == 0 {
		t.Fatal("expected !help to dispatch and reply via App.Chat")
	}
	for _, call := range ses.Calls {
		if strings.HasPrefix(call, "LoginIfNecessary") {
			t.Fatalf("gateway platform logged the sender in: %v", ses.Calls)
		}
	}
}

func TestHandleMessage_GatewayPlatformNonCommandIsQuiet(t *testing.T) {
	app, rec, _ := gatewayPlatformApp(t)

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
func TestHandleMessage_TwitchLogsTheSenderIn(t *testing.T) {
	app := newTestApp(video.Video{})
	app.Platform = platformTwitch
	app.Chat = &recordingChat{}
	ses := &recordingSessions{}
	app.Sessions = ses

	app.HandleMessage(context.Background(), IncomingMessage{User: "TwitchViewer", Text: "nice stream"})

	if !slices.Contains(ses.Calls, `LoginIfNecessary("TwitchViewer")`) {
		t.Errorf("Twitch inbound did not log the sender in; calls = %v", ses.Calls)
	}
}
