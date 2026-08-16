package chatbot

import (
	"context"
	"testing"
)

// recordingChatCounter counts Add calls.
type recordingChatCounter struct{ adds int }

func (r *recordingChatCounter) Add() { r.adds++ }

// Every inbound message lands on the counter — plain chatter and commands
// alike, since the chat-rate sample is a total, not a per-kind breakdown.
func TestHandleMessage_TalliesEveryMessage(t *testing.T) {
	app, _ := gatewayPlatformApp(t)
	rec := &recordingChatCounter{}
	app.ChatCounter = rec

	app.HandleMessage(context.Background(), IncomingMessage{User: "YouTubeViewer", Text: "nice stream"})
	app.HandleMessage(context.Background(), IncomingMessage{User: "YouTubeViewer", Text: "!help"})

	if rec.adds != 2 {
		t.Errorf("counter saw %d messages, want 2", rec.adds)
	}
}
