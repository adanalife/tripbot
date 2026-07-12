// Package achievements awards viewer achievements from what's on screen.
// The Twitch instance's video-change hook calls HandleVideoChange with the
// new clip and the viewers currently in chat; it upserts each viewer's
// (state, day) visit row and inserts any newly-crossed achievements. (The
// append-only "what was on screen when" record is pkg/viewstats's
// video_plays log, written on the same hook.) Awarding is INSERT ... ON
// CONFLICT DO NOTHING against the achievements table's unique key, so every
// step is idempotent — a missed tick self-heals on the next clip change.
package achievements

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/video"
	"gorm.io/gorm"
)

// visitTiers are the day-counts at which a state-visit achievement unlocks,
// with the display title's prefix. A "visit" is a distinct calendar day (DB
// time, UTC) on which the viewer caught footage from that state.
var visitTiers = []struct {
	Days  int
	Title string // fmt template, gets the state name
}{
	{1, "First visit to %s"},
	{10, "10th visit to %s"},
	{100, "100th visit to %s"},
}

// landmarks the footage is known to pass. Radii are hand-tuned "the road
// actually gets this close" values, not visibility ranges — Devils Tower is
// wide because the approach road shows it for miles.
var landmarks = []struct {
	Key      string
	Title    string
	Lat, Lng float64
	RadiusKm float64
}{
	{"old-faithful", "Saw Old Faithful", 44.4605, -110.8281, 2},
	{"golden-gate-bridge", "Saw the Golden Gate Bridge", 37.8199, -122.4786, 3},
	{"devils-tower", "Saw Devils Tower", 44.5902, -104.7146, 10},
}

// award inserts one achievement row, idempotently. Returns true when the row
// is new (i.e. this is the moment to announce it).
func award(tx *gorm.DB, username, name, title string) (bool, error) {
	res := tx.Exec(`INSERT INTO achievements (platform, username, name, title)
		VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		c.Conf.Platform, username, name, title)
	return res.RowsAffected > 0, res.Error
}

// stateVisitSQL awards one tier for one state across all users at once: any
// user whose distinct-day count for the state has reached the tier gets the
// row. Set-based and viewer-independent so a tick that died between the day
// upsert and the award still converges next clip change.
const stateVisitSQL = `
INSERT INTO achievements (platform, username, name, title)
SELECT platform, username, ?, ?
FROM user_state_days
WHERE platform = ? AND state = ?
GROUP BY platform, username
HAVING COUNT(*) >= ?
ON CONFLICT DO NOTHING
RETURNING username
`

// HandleVideoChange is the Twitch instance's clip-transition hook. viewers is
// everyone currently in chat (bots already excluded). It returns the chat
// announcements for any achievements unlocked by this clip; the caller owns
// actually saying them.
func HandleVideoChange(ctx context.Context, v video.Video, viewers []string) []string {
	if c.Conf.ReadOnly || v.ID == 0 {
		return nil
	}

	// unlocked collects title -> usernames for the announcement lines.
	unlocked := map[string][]string{}

	err := database.GormDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if v.State != "" {
			for _, viewer := range viewers {
				if err := tx.Exec(`INSERT INTO user_state_days (platform, username, state, day)
					VALUES (?, ?, ?, CURRENT_DATE) ON CONFLICT DO NOTHING`,
					c.Conf.Platform, viewer, v.State).Error; err != nil {
					return err
				}
			}
			for _, tier := range visitTiers {
				name := fmt.Sprintf("state-%s-%d", slugify(v.State), tier.Days)
				title := fmt.Sprintf(tier.Title, v.State)
				var rows []struct{ Username string }
				if err := tx.Raw(stateVisitSQL, name, title, c.Conf.Platform, v.State, tier.Days).
					Scan(&rows).Error; err != nil {
					return err
				}
				for _, r := range rows {
					unlocked[title] = append(unlocked[title], r.Username)
				}
			}
		}

		// Landmark sightings need a trustworthy GPS fix on the clip.
		if !v.Flagged && (v.Lat != 0 || v.Lng != 0) {
			for _, lm := range landmarks {
				if distanceKm(v.Lat, v.Lng, lm.Lat, lm.Lng) > lm.RadiusKm {
					continue
				}
				for _, viewer := range viewers {
					isNew, err := award(tx, viewer, "landmark-"+lm.Key, lm.Title)
					if err != nil {
						return err
					}
					if isNew {
						unlocked[lm.Title] = append(unlocked[lm.Title], viewer)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		slog.ErrorContext(ctx, "achievement processing failed", "err", err, "file", v.File())
		return nil
	}

	var msgs []string
	for title, usernames := range unlocked {
		for i, u := range usernames {
			usernames[i] = "@" + u
		}
		msgs = append(msgs, fmt.Sprintf("🏆 Achievement unlocked — %s: %s",
			title, strings.Join(usernames, ", ")))
	}
	return msgs
}

func slugify(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), " ", "-")
}

// distanceKm is the haversine great-circle distance.
func distanceKm(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusKm = 6371
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusKm * math.Asin(math.Sqrt(a))
}
