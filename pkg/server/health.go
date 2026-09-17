package server

import (
	"context"
	"errors"

	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/httpmw"
	"github.com/adanalife/tripbot/pkg/natsclient"
)

// depChecks are the hard dependencies /health/deps reports on. Postgres is
// unconditional — nothing tripbot does survives losing it. NATS is checked
// only when configured: NATS_URL is unset on a laptop, and an unconfigured
// dependency is not a degraded one.
func (s *Server) depChecks() []httpmw.ReadyCheck {
	checks := []httpmw.ReadyCheck{{Name: "postgres", Fn: database.Ping}}
	if s.cfg.NatsURL != "" {
		checks = append(checks, httpmw.ReadyCheck{Name: "nats", Fn: natsPing})
	}
	return checks
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
