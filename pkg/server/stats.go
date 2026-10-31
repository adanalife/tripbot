package server

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/adanalife/tripbot/pkg/database"
)

// The stats endpoints serve the console's broad stats page: lifetime totals
// over the whole log, a recent playback window, and community numbers. Same
// posture as the insights endpoints — read-only, internal-only, fleet-wide
// across platforms, bots excluded wherever usernames are counted or shown.
// The windowed queries ride the events_event_date / viewstats date indexes
// (migrations 048/049); the lifetime queries are whole-table aggregates, so
// they are served from an hour-long cache.

const (
	playbackStatsDefaultDays  = 7
	communityStatsDefaultDays = 30
	songStatsDefaultDays      = 90
	songStatsLimit            = 20
)

// eventKindCount is one event kind's all-time row count — a census of the
// log itself, so bots and sentinel-dated rows are included.
type eventKindCount struct {
	Event string `json:"event"`
	Count int64  `json:"count"`
}

// lifetimeUsers aggregates user_rollups through the users join (bots out),
// with the bot population reported separately.
type lifetimeUsers struct {
	Known       int64   `json:"known"`
	Bots        int64   `json:"bots"`
	Sessions    int64   `json:"sessions"`
	EventsMiles float64 `json:"events_miles"`
	ExtraMiles  float64 `json:"extra_miles"`
}

// lifetimeCorpus describes the footage corpus. Hours is null: videos carries
// no duration column, and the field stays in the shape for the day one exists.
type lifetimeCorpus struct {
	Clips  int64    `json:"clips"`
	States int64    `json:"states"`
	Hours  *float64 `json:"hours"`
}

// peakChatters is the all-time viewer_samples high-water mark. The earliest
// sample wins a tie, so the answer is stable as new equal peaks accrue.
type peakChatters struct {
	Count int64     `json:"count"`
	At    time.Time `json:"at"`
}

// lifetimeStatsResponse is the wire shape of GET /api/stats/lifetime. Since
// and PeakChatters are null when the log/samples are empty; EventKinds is
// always an array.
type lifetimeStatsResponse struct {
	Since        *time.Time       `json:"since"`
	EventsTotal  int64            `json:"events_total"`
	EventKinds   []eventKindCount `json:"event_kinds"`
	Users        lifetimeUsers    `json:"users"`
	Corpus       lifetimeCorpus   `json:"corpus"`
	PeakChatters *peakChatters    `json:"peak_chatters"`
}

// sinceSQL finds the oldest real event, skipping the 0001-01-01 rows the
// date_created insert bug left — the same sentinel guard pkg/events and
// pkg/rollups apply.
const sinceSQL = `
SELECT MIN(date_created) FROM events WHERE date_created > '2000-01-01'`

const eventKindsSQL = `
SELECT event, COUNT(*) AS count
FROM events
GROUP BY event
ORDER BY count DESC, event`

// lifetimeUsersSQL reads the reconciler's per-user aggregates. user_rollups
// includes bots by design; the users join is where every reader filters them.
const lifetimeUsersSQL = `
SELECT COUNT(*) AS known,
       COALESCE(SUM(r.session_count), 0) AS sessions,
       ROUND(COALESCE(SUM(r.events_miles), 0)::numeric, 1)::float8 AS events_miles,
       ROUND(COALESCE(SUM(r.extra_miles), 0)::numeric, 1)::float8  AS extra_miles
FROM user_rollups r
JOIN users u ON u.platform = r.platform AND u.username = r.username
WHERE u.is_bot = false`

const botCountSQL = `SELECT COUNT(*) FROM users WHERE is_bot = true`

const corpusSQL = `
SELECT COUNT(*) AS clips,
       COUNT(DISTINCT NULLIF(state, '')) AS states
FROM videos`

const peakChattersSQL = `
SELECT count, sampled_at AS at
FROM viewer_samples
ORDER BY count DESC, sampled_at
LIMIT 1`

// lifetimeTTL is how long a gathered answer is reused. The figures are
// all-time totals over an append-only log, so an hour of drift is a rounding
// error against numbers that only grow.
const lifetimeTTL = time.Hour

// lifetimeStatsSource is the data seam the lifetime endpoint reads through,
// overridable in tests so the cache is exercised without a DB.
var lifetimeStatsSource = gatherLifetimeStats

// lifetimeCache holds the gathered answer so the whole-table scans behind it
// run once an hour rather than once per page load. sinceSQL has no index to
// ride and the kind census is a GROUP BY over every row, so the cost grows
// with the log forever while the answer barely moves.
var lifetimeCache struct {
	sync.Mutex
	stats lifetimeStatsResponse
	// builtAt is zero until the first successful gather; a failed query isn't
	// cached, so a DB blip doesn't pin an error for an hour.
	builtAt time.Time
}

// cachedLifetimeStats returns the lifetime figures, gathering them if the
// cache is cold or stale.
func cachedLifetimeStats(ctx context.Context) (lifetimeStatsResponse, error) {
	lifetimeCache.Lock()
	defer lifetimeCache.Unlock()
	if !lifetimeCache.builtAt.IsZero() && time.Since(lifetimeCache.builtAt) < lifetimeTTL {
		return lifetimeCache.stats, nil
	}
	stats, err := lifetimeStatsSource(ctx)
	if err != nil {
		return stats, err
	}
	lifetimeCache.stats, lifetimeCache.builtAt = stats, time.Now()
	return stats, nil
}

func gatherLifetimeStats(ctx context.Context) (lifetimeStatsResponse, error) {
	out := lifetimeStatsResponse{EventKinds: []eventKindCount{}}
	db := database.GormDB().WithContext(ctx)

	var since sql.NullTime
	if err := db.Raw(sinceSQL).Scan(&since).Error; err != nil {
		return out, fmt.Errorf("since: %w", err)
	}
	if since.Valid {
		t := since.Time.UTC()
		out.Since = &t
	}

	if err := db.Raw(eventKindsSQL).Scan(&out.EventKinds).Error; err != nil {
		return out, fmt.Errorf("event kinds: %w", err)
	}
	// The kind census already visits every row, so the grand total is its sum
	// rather than a second whole-table count.
	for _, k := range out.EventKinds {
		out.EventsTotal += k.Count
	}

	if err := db.Raw(lifetimeUsersSQL).Scan(&out.Users).Error; err != nil {
		return out, fmt.Errorf("user rollups: %w", err)
	}
	if err := db.Raw(botCountSQL).Scan(&out.Users.Bots).Error; err != nil {
		return out, fmt.Errorf("bot count: %w", err)
	}

	if err := db.Raw(corpusSQL).Scan(&out.Corpus).Error; err != nil {
		return out, fmt.Errorf("corpus: %w", err)
	}

	var peak peakChatters
	res := db.Raw(peakChattersSQL).Scan(&peak)
	if res.Error != nil {
		return out, fmt.Errorf("peak chatters: %w", res.Error)
	}
	if res.RowsAffected > 0 {
		peak.At = peak.At.UTC()
		out.PeakChatters = &peak
	}
	return out, nil
}

// playbackStatsResponse is the wire shape of GET /api/stats/playback: what
// the playhead and its audience did over the window. Timewarps, guesses and
// commands read event kinds whose writers land separately; they stay zero
// until those deploy.
type playbackStatsResponse struct {
	Days                int   `json:"days"`
	Plays               int64 `json:"plays"`
	DistinctClips       int64 `json:"distinct_clips"`
	Crossings           int64 `json:"crossings"`
	SequentialCrossings int64 `json:"sequential_crossings"`
	Timewarps           int64 `json:"timewarps"`
	Guesses             int64 `json:"guesses"`
	Commands            int64 `json:"commands"`
}

// playbackSQL bundles the window's counters into one round trip.
// state_crossing is a system event (empty username), so it takes no users
// join; the viewer-attributed kinds exclude bots the same way the insights
// queries do.
const playbackSQL = `
SELECT
  (SELECT COUNT(*) FROM video_plays
     WHERE started_at >= now() - make_interval(days => @days)) AS plays,
  (SELECT COUNT(DISTINCT video_id) FROM video_plays
     WHERE video_id IS NOT NULL
       AND started_at >= now() - make_interval(days => @days)) AS distinct_clips,
  (SELECT COUNT(*) FROM events
     WHERE event = 'state_crossing'
       AND date_created >= now() - make_interval(days => @days)) AS crossings,
  (SELECT COUNT(*) FROM events
     WHERE event = 'state_crossing'
       AND meta->>'sequential' = 'true'
       AND date_created >= now() - make_interval(days => @days)) AS sequential_crossings,
  (SELECT COUNT(*) FROM events e
     JOIN users u ON u.platform = e.platform AND u.username = e.username
     WHERE e.event = 'timewarp' AND u.is_bot = false
       AND e.date_created >= now() - make_interval(days => @days)) AS timewarps,
  (SELECT COUNT(*) FROM events e
     JOIN users u ON u.platform = e.platform AND u.username = e.username
     WHERE e.event = 'guess_submitted' AND u.is_bot = false
       AND e.date_created >= now() - make_interval(days => @days)) AS guesses,
  (SELECT COUNT(*) FROM events e
     JOIN users u ON u.platform = e.platform AND u.username = e.username
     WHERE e.event = 'command_run' AND u.is_bot = false
       AND e.date_created >= now() - make_interval(days => @days)) AS commands`

func gatherPlaybackStats(ctx context.Context, days int) (playbackStatsResponse, error) {
	out := playbackStatsResponse{Days: days}
	err := database.GormDB().WithContext(ctx).
		Raw(playbackSQL, sql.Named("days", days)).Scan(&out).Error
	if err != nil {
		return out, fmt.Errorf("playback: %w", err)
	}
	out.Days = days
	return out, nil
}

// topMilesEntry is one leaderboard row from users.miles — the authoritative
// display number, not the events-derived audit sum.
type topMilesEntry struct {
	Username string  `json:"username"`
	Platform string  `json:"platform"`
	Miles    float64 `json:"miles"`
}

// boardRow is one frozen scoreboard_snapshots placement.
type boardRow struct {
	Rank     int     `json:"rank"`
	Username string  `json:"username"`
	Value    float64 `json:"value"`
}

// boardSnapshot is one board's top placements from the most recent
// snapshotted month.
type boardSnapshot struct {
	Board string     `json:"board"`
	Rows  []boardRow `json:"rows"`
}

// communityStatsResponse is the wire shape of GET /api/stats/community.
type communityStatsResponse struct {
	Days         int             `json:"days"`
	Subscribes   int64           `json:"subscribes"`
	Unsubscribes int64           `json:"unsubscribes"`
	Corrections  int64           `json:"corrections"`
	TopMiles     []topMilesEntry `json:"top_miles"`
	Boards       []boardSnapshot `json:"boards"`
}

const communityCountsSQL = `
SELECT
  COUNT(*) FILTER (WHERE e.event = 'subscribe')   AS subscribes,
  COUNT(*) FILTER (WHERE e.event = 'unsubscribe') AS unsubscribes,
  COUNT(*) FILTER (WHERE e.event = 'correction')  AS corrections
FROM events e
JOIN users u ON u.platform = e.platform AND u.username = e.username
WHERE e.event IN ('subscribe', 'unsubscribe', 'correction')
  AND e.date_created >= now() - make_interval(days => @days)
  AND u.is_bot = false`

// topMilesSQL mirrors the leaderboard-eligibility filter pkg/rollups'
// snapshot write applies: no bots, no opted-out accounts.
const topMilesSQL = `
SELECT username, platform, ROUND(miles::numeric, 1)::float8 AS miles
FROM users
WHERE is_bot = false AND exclude_from_leaderboard = false
ORDER BY miles DESC, username
LIMIT 10`

// latestBoardsSQL reads one platform's most recent snapshotted month's
// boards, top 5 placements each. Snapshots rank each platform separately
// (UNIQUE(scoreboard_name, platform, rank)), so the boards are read for the
// asking instance's platform, like /api/leaderboards — mixing them would list
// two players at every rank. Board names embed their month as a _YYYY_MM suffix
// (miles_2026_07, guess_state_2026_07), which is what "most recent" keys on —
// snapshot rows are written once at rollover, so date_created would work too,
// but the name is the identity the boards are queried by everywhere else.
// Eligibility was applied when the snapshot froze; rows here are read back
// verbatim.
const latestBoardsSQL = `
WITH latest AS (
    SELECT MAX(substring(scoreboard_name FROM '[0-9]{4}_[0-9]{2}$')) AS month
    FROM scoreboard_snapshots
    WHERE platform = @platform
)
SELECT s.scoreboard_name AS board,
       s.rank,
       s.username,
       ROUND(s.value::numeric, 1)::float8 AS value
FROM scoreboard_snapshots s, latest
WHERE substring(s.scoreboard_name FROM '[0-9]{4}_[0-9]{2}$') = latest.month
  AND s.platform = @platform
  AND s.rank <= 5
ORDER BY s.scoreboard_name, s.rank`

func gatherCommunityStats(ctx context.Context, days int, platform string) (communityStatsResponse, error) {
	out := communityStatsResponse{
		Days:     days,
		TopMiles: []topMilesEntry{},
		Boards:   []boardSnapshot{},
	}
	db := database.GormDB().WithContext(ctx)

	var counts struct {
		Subscribes   int64
		Unsubscribes int64
		Corrections  int64
	}
	if err := db.Raw(communityCountsSQL, sql.Named("days", days)).Scan(&counts).Error; err != nil {
		return out, fmt.Errorf("community counts: %w", err)
	}
	out.Subscribes, out.Unsubscribes, out.Corrections =
		counts.Subscribes, counts.Unsubscribes, counts.Corrections

	if err := db.Raw(topMilesSQL).Scan(&out.TopMiles).Error; err != nil {
		return out, fmt.Errorf("top miles: %w", err)
	}

	var rows []struct {
		Board    string
		Rank     int
		Username string
		Value    float64
	}
	if err := db.Raw(latestBoardsSQL, sql.Named("platform", platform)).Scan(&rows).Error; err != nil {
		return out, fmt.Errorf("boards: %w", err)
	}
	// Rows arrive grouped by board name; fold consecutive rows into one
	// boardSnapshot each.
	for _, r := range rows {
		n := len(out.Boards)
		if n == 0 || out.Boards[n-1].Board != r.Board {
			out.Boards = append(out.Boards, boardSnapshot{Board: r.Board, Rows: []boardRow{}})
			n++
		}
		out.Boards[n-1].Rows = append(out.Boards[n-1].Rows,
			boardRow{Rank: r.Rank, Username: r.Username, Value: r.Value})
	}
	return out, nil
}

// lifetimeStatsHandler serves GET /api/stats/lifetime: whole-log totals, the
// user population, the footage corpus, and the all-time chatter peak. The
// answer is cached an hour (lifetimeTTL).
func lifetimeStatsHandler(w http.ResponseWriter, r *http.Request) {
	payload, err := cachedLifetimeStats(r.Context())
	if err != nil {
		logQueryFailure(r.Context(), "lifetime stats query failed", err)
		insightsError(w, "couldn't gather lifetime stats")
		return
	}
	writeInsights(w, r, payload)
}

// playbackStatsHandler serves GET /api/stats/playback: plays, crossings and
// viewer-action counts over the ?days window.
func playbackStatsHandler(w http.ResponseWriter, r *http.Request) {
	days := insightsDays(r, playbackStatsDefaultDays)
	payload, err := gatherPlaybackStats(r.Context(), days)
	if err != nil {
		logQueryFailure(r.Context(), "playback stats query failed", err, "days", days)
		insightsError(w, "couldn't gather playback stats")
		return
	}
	writeInsights(w, r, payload)
}

// playbackDailyResponse is the wire shape of GET /api/stats/playback/daily:
// the playback counters per day over the ?days window, oldest first, with a
// row for every day whether or not anything happened — so a chart's x-axis is
// the calendar and a quiet day reads as zero rather than vanishing.
type playbackDailyResponse struct {
	Days    int           `json:"days"`
	Buckets []playbackDay `json:"buckets"`
}

// playbackDay is one day's counters, the kinds playbackStatsResponse totals.
// Days are the database session's calendar days (UTC on the cluster).
type playbackDay struct {
	Day       string `json:"day"`
	Plays     int64  `json:"plays"`
	Crossings int64  `json:"crossings"`
	Timewarps int64  `json:"timewarps"`
	Guesses   int64  `json:"guesses"`
	Commands  int64  `json:"commands"`
	// Visits is the closed human visits that began that day, and
	// MedianMinutes how long the middle one lasted (0 on a day with none) —
	// the stays block's figures as a series, so "are visits getting longer?"
	// has a trend to read.
	Visits        int64   `json:"visits"`
	MedianMinutes float64 `json:"median_minutes"`
}

// The daily series defaults to the whole clamp: a trend wants the long view,
// and 90 small rows is a small payload.
const playbackDailyDefaultDays = insightsMaxDays

// playbackDailySQL counts each kind per day and joins the counts onto a
// generated calendar, so every day in the window is a row. `since` is the
// first day's midnight; the same bot exclusion as playbackSQL applies to the
// viewer-attributed kinds, and the visits are sessionsSQL's, by login day.
const playbackDailySQL = `
WITH days AS (
    SELECT generate_series(CAST(@since AS date), CAST(@since AS date) + (CAST(@days AS int) - 1), interval '1 day')::date AS day
),
` + visitsCTE + `,
stays AS (
    SELECT day,
           COUNT(*) AS n,
           ROUND(percentile_cont(0.5) WITHIN GROUP (ORDER BY minutes)::numeric, 1)::float8 AS median_minutes
    FROM visits
    GROUP BY 1
),
plays AS (
    SELECT started_at::date AS day, COUNT(*) AS n
    FROM video_plays
    WHERE started_at >= @since
    GROUP BY 1
),
crossings AS (
    SELECT date_created::date AS day, COUNT(*) AS n
    FROM events
    WHERE event = 'state_crossing'
      AND date_created >= @since
    GROUP BY 1
),
actions AS (
    SELECT e.date_created::date AS day, e.event, COUNT(*) AS n
    FROM events e
    JOIN users u ON u.platform = e.platform AND u.username = e.username
    WHERE e.event IN ('timewarp', 'guess_submitted', 'command_run')
      AND e.date_created >= @since
      AND u.is_bot = false
    GROUP BY 1, 2
)
SELECT to_char(d.day, 'YYYY-MM-DD') AS day,
       COALESCE(p.n, 0) AS plays,
       COALESCE(c.n, 0) AS crossings,
       COALESCE(t.n, 0) AS timewarps,
       COALESCE(g.n, 0) AS guesses,
       COALESCE(m.n, 0) AS commands,
       COALESCE(s.n, 0) AS visits,
       COALESCE(s.median_minutes, 0) AS median_minutes
FROM days d
LEFT JOIN stays s     ON s.day = d.day
LEFT JOIN plays p     ON p.day = d.day
LEFT JOIN crossings c ON c.day = d.day
LEFT JOIN actions t   ON t.day = d.day AND t.event = 'timewarp'
LEFT JOIN actions g   ON g.day = d.day AND g.event = 'guess_submitted'
LEFT JOIN actions m   ON m.day = d.day AND m.event = 'command_run'
ORDER BY d.day`

// playbackDailySince is midnight (UTC) of the first day in a window of
// `days` days ending today.
func playbackDailySince(days int, now time.Time) time.Time {
	y, m, d := now.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(days - 1))
}

func gatherPlaybackDaily(ctx context.Context, days int) (playbackDailyResponse, error) {
	out := playbackDailyResponse{Days: days, Buckets: []playbackDay{}}
	err := database.GormDB().WithContext(ctx).
		Raw(playbackDailySQL,
			sql.Named("since", playbackDailySince(days, time.Now())),
			sql.Named("days", days)).
		Scan(&out.Buckets).Error
	if err != nil {
		return out, fmt.Errorf("playback daily: %w", err)
	}
	return out, nil
}

// playbackDailyHandler serves GET /api/stats/playback/daily: the playback
// counters per day over the ?days window, for a trend beside the totals.
func playbackDailyHandler(w http.ResponseWriter, r *http.Request) {
	days := insightsDays(r, playbackDailyDefaultDays)
	payload, err := gatherPlaybackDaily(r.Context(), days)
	if err != nil {
		logQueryFailure(r.Context(), "playback daily query failed", err, "days", days)
		insightsError(w, "couldn't gather the daily playback counts")
		return
	}
	writeInsights(w, r, payload)
}

// communityStatsHandler serves GET /api/stats/community: subscription and
// correction counts over the ?days window, the lifetime miles top 10, and
// the latest month's frozen scoreboards for this instance's platform.
func (s *Server) communityStatsHandler(w http.ResponseWriter, r *http.Request) {
	days := insightsDays(r, communityStatsDefaultDays)
	payload, err := gatherCommunityStats(r.Context(), days, s.cfg.Platform)
	if err != nil {
		logQueryFailure(r.Context(), "community stats query failed", err, "days", days)
		insightsError(w, "couldn't gather community stats")
		return
	}
	writeInsights(w, r, payload)
}

// songCount is one track's tally of !song answers that named it.
type songCount struct {
	Title     string    `json:"title"`
	Artist    string    `json:"artist"`
	Count     int64     `json:"count"`
	Askers    int64     `json:"askers"`
	LastAsked time.Time `json:"last_asked"`
}

// songStatsResponse is the wire shape of GET /api/stats/songs.
type songStatsResponse struct {
	Days  int         `json:"days"`
	Songs []songCount `json:"songs"`
}

// songStatsSQL ranks the tracks !song named, read from the detail its
// command_run rows carry. A run that named no track (the car hum, a failed
// feed fetch) has no detail title and drops out.
const songStatsSQL = `
SELECT e.meta->'detail'->>'title'                 AS title,
       COALESCE(e.meta->'detail'->>'artist', '') AS artist,
       COUNT(*)                                   AS count,
       COUNT(DISTINCT e.username)                 AS askers,
       MAX(e.date_created)                        AS last_asked
FROM events e
JOIN users u ON u.platform = e.platform AND u.username = e.username
WHERE e.event = 'command_run'
  AND e.meta->>'command' = '!song'
  AND e.meta->'detail'->>'title' IS NOT NULL
  AND e.date_created >= now() - make_interval(days => @days)
  AND u.is_bot = false
GROUP BY 1, 2
ORDER BY count DESC, last_asked DESC
LIMIT @limit`

// songStatsHandler serves GET /api/stats/songs: the tracks chat asked about
// most with !song over the ?days window.
func songStatsHandler(w http.ResponseWriter, r *http.Request) {
	days := insightsDays(r, songStatsDefaultDays)
	out := songStatsResponse{Days: days, Songs: []songCount{}}
	err := database.GormDB().WithContext(r.Context()).
		Raw(songStatsSQL, sql.Named("days", days), sql.Named("limit", songStatsLimit)).
		Scan(&out.Songs).Error
	if err != nil {
		logQueryFailure(r.Context(), "song stats query failed", err, "days", days)
		insightsError(w, "couldn't gather song stats")
		return
	}
	writeInsights(w, r, out)
}
