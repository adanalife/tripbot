package server

import (
	"context"
	"errors"

	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/httpmw"
	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/adanalife/tripbot/pkg/natsclient"
)

// depChecks are the hard dependencies /health/deps reports on. Postgres is
// unconditional — nothing tripbot does survives losing it. NATS is checked
// only when configured: NATS_URL is unset on a laptop, and an unconfigured
// dependency is not a degraded one. Chat is checked on the Twitch instance,
// the only one whose poller reports a chat connection (Twitch chat exists
// off-stream; elsewhere chat exists only while live, so "not in chat" there
// is just "offline"). EventSub likewise: only the Twitch instance dials it.
func (s *Server) depChecks() []httpmw.ReadyCheck {
	checks := []httpmw.ReadyCheck{{Name: "postgres", Fn: database.Ping}}
	if s.cfg.NatsURL != "" {
		checks = append(checks, httpmw.ReadyCheck{Name: "nats", Fn: natsPing})
	}
	if s.cfg.Platform == "twitch" {
		checks = append(checks, httpmw.ReadyCheck{Name: "chat", Fn: chatPing},
			httpmw.ReadyCheck{Name: "eventsub", Fn: eventsubPing})
	}
	return checks
}

// chatPing reports whether the bot is receiving chat, from the last inbound
// poll through the gateway. Before the first poll answers there is nothing to
// judge, which is not a failure.
func chatPing(context.Context) error {
	connected, reported := instrumentation.TwitchConnection.Connected()
	if reported && !connected {
		return errors.New("not receiving chat")
	}
	return nil
}

// eventsubPing reports whether an EventSub session is holding subscriptions,
// from the last subscribe round. A session that ended (the redial loop owns
// bringing it back) and a round Twitch wholly denied both read as down; before
// the first round there is nothing to judge, which is not a failure.
func eventsubPing(context.Context) error {
	up, reported := instrumentation.EventSubSubscriptions.Session()
	if reported && !up {
		return errors.New("no eventsub session")
	}
	return nil
}

// natsPing reports the singleton connection's state. A nil conn means Connect
// never succeeded; IsConnected false means it has since dropped and the client
// is reconnecting. Both are "not usable now", which is the question asked.
func natsPing(context.Context) error {
	conn := natsclient.Conn()
	if conn == nil {
		return errors.New("not connected")
	}
	if !conn.IsConnected() {
		return errors.New("connection lost, reconnecting")
	}
	return nil
}
