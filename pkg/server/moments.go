package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/adanalife/tripbot/pkg/database"
	"github.com/adanalife/tripbot/pkg/video"
	"github.com/gorilla/mux"
	"gorm.io/gorm"
)

// moment is one of the operator's marks on a clip, at Sec seconds into the
// clip as it airs now.
type moment struct {
	Kind string  `json:"kind"`
	Sec  float64 `json:"sec"`
	Note string  `json:"note"`
}

var momentKinds = map[string]bool{"star": true, "bad_coord": true}

// A clip's offset into its original recording, read off its own track: the
// two clocks differ by a constant within a clip. A clip with no track is
// taken as untrimmed.
// ponytail: a trimmed clip with no coords mis-keys its marks by its trim
// start; derive the offset from the trim manifest if one ever needs it.
const clipOffsetSQL = `
SELECT COALESCE((SELECT source_ts_sec - ts_sec FROM video_coords
                 WHERE video_id = @id AND ts_sec IS NOT NULL LIMIT 1), 0)`

func clipOffset(db *gorm.DB, id int) (float64, error) {
	var off float64
	err := db.Raw(clipOffsetSQL, sql.Named("id", id)).Scan(&off).Error
	return off, err
}

// momentsOf is clip id's marks in clip order, never nil, so it encodes as [].
func momentsOf(ctx context.Context, db *gorm.DB, id int) ([]moment, error) {
	off, err := clipOffset(db, id)
	if err != nil {
		return nil, err
	}
	out := []moment{}
	err = db.WithContext(ctx).Raw(`
SELECT kind, source_ts_sec - @off AS sec, note FROM video_moments
WHERE video_id = @id ORDER BY source_ts_sec, kind`,
		sql.Named("id", id), sql.Named("off", off)).Scan(&out).Error
	return out, err
}

const maxMomentNoteRunes = 200

// videoMomentsHandler serves POST /api/videos/{id}/moments: mark a moment of
// the clip (kind star or bad_coord at sec seconds into it, with an optional
// note), or with remove set, unmark it. Marking an already-marked moment
// replaces its note.
func videoMomentsHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil || id <= 0 {
		http.Error(w, `{"error":"bad video id"}`, http.StatusBadRequest)
		return
	}
	var body struct {
		Kind   string   `json:"kind"`
		Sec    *float64 `json:"sec"`
		Note   string   `json:"note"`
		Remove bool     `json:"remove"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
		!momentKinds[body.Kind] || body.Sec == nil || *body.Sec < 0 {
		http.Error(w, `{"error":"body must set kind (star or bad_coord) and sec"}`, http.StatusBadRequest)
		return
	}
	note := strings.TrimSpace(body.Note)
	if utf8.RuneCountInString(note) > maxMomentNoteRunes {
		http.Error(w, fmt.Sprintf(`{"error":"note is longer than %d characters"}`, maxMomentNoteRunes), http.StatusBadRequest)
		return
	}
	db := database.GormDB().WithContext(r.Context())
	var v video.Video
	if err := db.First(&v, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, `{"error":"no such video"}`, http.StatusNotFound)
		return
	}
	off, err := clipOffset(db, id)
	if err == nil {
		args := []any{sql.Named("id", id), sql.Named("kind", body.Kind),
			sql.Named("src", *body.Sec+off), sql.Named("note", note)}
		if body.Remove {
			err = db.Exec(`DELETE FROM video_moments
WHERE video_id = @id AND kind = @kind AND source_ts_sec = @src`, args...).Error
		} else {
			err = db.Exec(`INSERT INTO video_moments (video_id, kind, source_ts_sec, note)
VALUES (@id, @kind, @src, @note)
ON CONFLICT (video_id, kind, source_ts_sec) DO UPDATE SET note = EXCLUDED.note`, args...).Error
		}
	}
	if err != nil {
		logQueryFailure(r.Context(), "video moment write failed", err, "id", id)
		http.Error(w, `{"error":"couldn't write the moment"}`, http.StatusInternalServerError)
		return
	}
	slog.InfoContext(r.Context(), "video moment set via console", "id", id,
		"kind", body.Kind, "sec", *body.Sec, "remove", body.Remove)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}
