package events

import (
	"context"
	"time"

	"github.com/adanalife/tripbot/pkg/database"
)

// orphanLookback bounds how far back OrphanedSessions reads. Every rollup
// drops sessions over 24h as missed logouts, so an orphan older than a week
// cannot move a number anyone reads, and the bound keeps the scan off the
// bulk of an append-only table.
const orphanLookback = 7 * 24 * time.Hour

// orphanedSessionsSQL counts logins with no logout sharing their session_id.
// Both sides are windowed: a logout for a login in the window can only be at
// or after the window's start, so the anti-join never leaves the window.
// The all-zero UUID is what a row without a session writes (a non-pointer
// field), so it is excluded by value, not by NULL.
const orphanedSessionsSQL = `
SELECT COUNT(*) FROM events l
WHERE l.platform = ? AND l.event = 'login'
  AND l.session_id IS NOT NULL AND l.session_id <> '00000000-0000-0000-0000-000000000000'
  AND l.date_created >= ? AND l.date_created < ?
  AND NOT EXISTS (
    SELECT 1 FROM events o
    WHERE o.event = 'logout' AND o.session_id = l.session_id AND o.date_created >= ?
  )`

// OrphanedSessions counts the platform's logins that predate before and have
// no logout with the same session_id — the sessions a previous process left
// open by exiting without writing its logouts. A graceful shutdown pairs every
// login, so a non-zero count means the last exit was ungraceful and names its
// cost: viewers whose in-flight miles that exit discarded. Only rows younger
// than orphanLookback are read.
func OrphanedSessions(ctx context.Context, platform string, before time.Time) (int64, error) {
	since := before.Add(-orphanLookback)
	var n int64
	err := database.GormDB().WithContext(ctx).
		Raw(orphanedSessionsSQL, platform, since, before, since).
		Scan(&n).Error
	return n, err
}
