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
// is just "offline").
func (s *Server) depChecks() []httpmw.ReadyCheck {
	checks := []httpmw.ReadyCheck{{Name: "postgres", Fn: database.Ping}}
	if s.cfg.NatsURL != "" {
		checks = append(checks, httpmw.ReadyCheck{Name: "nats", Fn: natsPing})
	}
	if s.cfg.Platform == "twitch" {
		checks = append(checks, httpmw.ReadyCheck{Name: "chat", Fn: chatPing})
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
