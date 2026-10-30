package video

import (
	"context"
	"database/sql"

	"github.com/adanalife/tripbot/pkg/database"
	"gorm.io/gorm"
)

// A Mark is one mark on a moment of a clip: an operator's star or bad_coord
// from the console, or a viewer's star or trim from chat. Sec is seconds into
// the clip as it airs now; By and Platform name who set it from chat and are
// empty for the console's marks.
type Mark struct {
	VideoID  int
	Kind     string
	Sec      float64
	Note     string
	By       string
	Platform string
}

// A clip's offset into its original recording, read off its own track: the
// two clocks differ by a constant within a clip. A clip with no track is
// taken as untrimmed.
// ponytail: a trimmed clip with no coords mis-keys its marks by its trim
// start; derive the offset from the trim manifest if one ever needs it.
const clipOffsetSQL = `
SELECT COALESCE((SELECT source_ts_sec - ts_sec FROM video_coords
                 WHERE video_id = @id AND ts_sec IS NOT NULL LIMIT 1), 0)`

// ClipOffset is seconds from the start of clip id's original recording to the
// start of the clip, the constant that turns a clip second into the
// source_ts_sec marks are keyed on.
func ClipOffset(db *gorm.DB, id int) (float64, error) {
	var off float64
	err := db.Raw(clipOffsetSQL, sql.Named("id", id)).Scan(&off).Error
	return off, err
}

// SetMark writes m, keyed to the original recording so a re-trim moves no
// mark. Marking a moment its setter already marked replaces the note. A
// mark set from chat first clears that viewer's earlier mark of the same kind
// on the clip, so one viewer holds at most one star and one trim per clip and
// a repeated command moves the mark rather than piling more up.
func SetMark(ctx context.Context, m Mark) error {
	return database.GormDB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		off, err := ClipOffset(tx, m.VideoID)
		if err != nil {
			return err
		}
		args := []any{sql.Named("id", m.VideoID), sql.Named("kind", m.Kind),
			sql.Named("src", m.Sec+off), sql.Named("note", m.Note),
			sql.Named("by", m.By), sql.Named("platform", m.Platform)}
		if m.By != "" {
			if err := tx.Exec(`DELETE FROM video_moments
WHERE video_id = @id AND kind = @kind AND username = @by AND platform = @platform`, args...).Error; err != nil {
				return err
			}
		}
		return tx.Exec(`INSERT INTO video_moments (video_id, kind, source_ts_sec, note, username, platform)
VALUES (@id, @kind, @src, @note, @by, @platform)
ON CONFLICT (video_id, kind, source_ts_sec, username, platform)
DO UPDATE SET note = EXCLUDED.note`, args...).Error
	})
}

// RemoveMark takes the console's kind mark at sec seconds into clip id off,
// leaving viewers' marks on the same moment in place.
func RemoveMark(ctx context.Context, id int, kind string, sec float64) error {
	db := database.GormDB().WithContext(ctx)
	off, err := ClipOffset(db, id)
	if err != nil {
		return err
	}
	// source_ts_sec is REAL and comes back here through a float64 sum, so a
	// fractional second only matches within a tolerance.
	return db.Exec(`DELETE FROM video_moments
WHERE video_id = @id AND kind = @kind AND username = '' AND abs(source_ts_sec - @src) < 0.01`,
		sql.Named("id", id), sql.Named("kind", kind), sql.Named("src", sec+off)).Error
}

// The play before the platform's newest one, and how long it aired. ended_at
// is stamped by the play that superseded it, so it is NULL only when the bot
// never saw the clip leave.
const previousPlaySQL = `
SELECT video_id, EXTRACT(EPOCH FROM ended_at - started_at) AS aired
FROM video_plays
WHERE platform = @platform
ORDER BY started_at DESC
OFFSET 1 LIMIT 1`

// PreviousPlay returns the clip that aired on platform before the one on
// screen and how many seconds of it aired. ok is false when there is no such
// play, or its clip or end went unrecorded.
func PreviousPlay(ctx context.Context, platform string) (id int, aired float64, ok bool, err error) {
	var row struct {
		VideoID *int
		Aired   *float64
	}
	err = database.GormDB().WithContext(ctx).Raw(previousPlaySQL, sql.Named("platform", platform)).Scan(&row).Error
	if err != nil || row.VideoID == nil || row.Aired == nil {
		return 0, 0, false, err
	}
	return *row.VideoID, *row.Aired, true, nil
}
