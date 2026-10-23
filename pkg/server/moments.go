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

// moment is one mark on a clip, at Sec seconds into the clip as it airs now.
// By and Platform name the viewer who set it from chat, empty for the
// operator's own marks.
type moment struct {
	Kind     string  `json:"kind"`
	Sec      float64 `json:"sec"`
	Note     string  `json:"note"`
	By       string  `json:"by"`
	Platform string  `json:"platform"`
}

// momentKinds are the kinds the console may set; a trim only comes from chat.
var momentKinds = map[string]bool{"star": true, "bad_coord": true}

// momentsOf is clip id's marks in clip order, never nil, so it encodes as [].
func momentsOf(ctx context.Context, db *gorm.DB, id int) ([]moment, error) {
	off, err := video.ClipOffset(db, id)
	if err != nil {
		return nil, err
	}
	out := []moment{}
	err = db.WithContext(ctx).Raw(`
SELECT kind, source_ts_sec - @off AS sec, note, username AS by, platform FROM video_moments
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
	m := video.Mark{VideoID: id, Kind: body.Kind, Sec: *body.Sec, Note: note}
	if body.Remove {
		err = video.RemoveMark(r.Context(), id, body.Kind, *body.Sec)
	} else {
		err = video.SetMark(r.Context(), m)
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
