package events

import (
	"context"
	"database/sql"
	"time"

	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/helpers"
)

// MaxSessionLength is the longest login→logout pair counted as watching; a
// longer one is a missed logout and earns nothing. pkg/rollups' reconcile
// applies the same cap.
const MaxSessionLength = 24 * time.Hour

// milesGainedSQL returns, per user, the seconds of paired session time inside
// [@since, @until] and the extra_miles_earned dated inside it. Pairing follows
// pkg/rollups' reconcile: a login pairs with the logout carrying its
// session_id (MIN collapsing a doubled logout), and a pair of
// MaxSessionLength or more is dropped. A user's latest login with no logout is
// the session still running and counts up to @until; an earlier unpaired login
// was superseded, so it pairs with nothing.
// A crashed login never followed by another reads as running until it ages past MaxSessionLength.
//
// Rows are read from MaxSessionLength before @since (the oldest login a counted
// session can have), for the named users only, which the
// events_username_event_date index serves. Legacy rows with no session_id
// (written before it existed) are not paired here — reconcile pairs those by
// order instead, so over a window reaching back that far the two disagree.
const milesGainedSQL = `
WITH recent AS (
    SELECT username, event, session_id, date_created, extra_miles_earned
    FROM events
    WHERE platform = @platform
      AND username IN @usernames
      AND event IN ('login', 'logout', 'correction')
      AND date_created >= CAST(@since AS timestamptz) - make_interval(secs => @cap)
      AND date_created <= CAST(@until AS timestamptz)
),
keyed AS (
    SELECT username,
           MIN(date_created) FILTER (WHERE event = 'login') AS login_time,
           MIN(date_created) FILTER (WHERE event = 'logout') AS logout_time
    FROM recent
    WHERE event IN ('login', 'logout')
      AND session_id <> '00000000-0000-0000-0000-000000000000'
    GROUP BY username, session_id
),
latest AS (
    SELECT username, MAX(login_time) AS login_time FROM keyed GROUP BY username
),
spans AS (
    SELECT username,
           GREATEST(login_time, CAST(@since AS timestamptz)) AS start, logout_time AS stop
    FROM keyed
    WHERE logout_time > login_time
      AND logout_time - login_time < make_interval(secs => @cap)
    UNION ALL
    SELECT k.username,
           GREATEST(k.login_time, CAST(@since AS timestamptz)), CAST(@until AS timestamptz)
    FROM keyed k
    JOIN latest l ON l.username = k.username AND l.login_time = k.login_time
    WHERE k.logout_time IS NULL
      AND CAST(@until AS timestamptz) - k.login_time < make_interval(secs => @cap)
),
gained AS (
    SELECT username, EXTRACT(EPOCH FROM (stop - start)) AS seconds, 0 AS extra
    FROM spans WHERE stop > start
    UNION ALL
    SELECT username, 0, extra_miles_earned
    FROM recent
    WHERE event IN ('logout', 'correction') AND extra_miles_earned IS NOT NULL
      AND date_created >= CAST(@since AS timestamptz)
)
SELECT username, SUM(seconds)::float8 AS seconds, SUM(extra)::float8 AS extra
FROM gained GROUP BY username`

// MilesGained returns the display miles each of usernames earned on platform
// between since and until, in one query: paired session time clipped to the
// window (see milesGainedSQL), converted at helpers.DurationToMiles' rate,
// plus extra_miles_earned from logout and correction rows inside it. A user
// who earned nothing is absent.
func MilesGained(ctx context.Context, platform string, usernames []string, since, until time.Time) (map[string]float64, error) {
	out := map[string]float64{}
	if len(usernames) == 0 {
		return out, nil
	}
	var rows []struct {
		Username string
		Seconds  float64
		Extra    float64
	}
	err := database.GormDB().WithContext(ctx).Raw(milesGainedSQL,
		sql.Named("platform", platform), sql.Named("usernames", usernames),
		sql.Named("since", since), sql.Named("until", until),
		sql.Named("cap", MaxSessionLength.Seconds())).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		session := time.Duration(r.Seconds * float64(time.Second))
		out[r.Username] = float64(helpers.DurationToMiles(session)) + r.Extra
	}
	return out, nil
}
