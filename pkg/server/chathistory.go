package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/adanalife/tripbot/pkg/database"
)

// The chat history endpoint pages the chat log (chat_messages) newest-first
// for the console's /history page. Each line carries the same keys as a
// chat.message on the bus, so the console renders it with its live-log code,
// plus the clip's state and city so a line can say where the van was.

const (
	chatHistoryDefaultLines = 200
	chatHistoryMaxLines     = 1000
)

// chatHistoryLine is one logged line, keyed like the bus envelope.
type chatHistoryLine struct {
	ID         int64    `json:"id"`
	Platform   string   `json:"platform"`
	Username   string   `json:"username"`
	UserID     string   `json:"user_id,omitempty"`
	MessageID  string   `json:"message_id,omitempty"`
	Text       string   `json:"text"`
	EmittedAt  string   `json:"emitted_at"`
	VideoID    *int     `json:"video_id,omitempty"`
	VideoFile  string   `json:"video_file,omitempty"`
	VideoTsSec *float64 `json:"video_ts_sec,omitempty"`
	VideoState string   `json:"video_state,omitempty"`
	VideoCity  string   `json:"video_city,omitempty"`
}

// A page: lines oldest-first, and the cursor for the page before them (0
// when this page reached the start of the log).
type chatHistoryPage struct {
	Lines      []chatHistoryLine `json:"lines"`
	NextBefore int64             `json:"next_before"`
}

// Newest first within the WHERE, then flipped in Go so a page reads top-down
// like the log it extends. @before = 0 means "from the newest"; @platform =
// ” means every platform. The clip join is LEFT: a line with no stamp still
// has its text.
const chatHistorySQL = `
SELECT m.id, m.platform, m.username, m.user_id, m.message_id, m.text, m.emitted_at,
       m.video_id, m.video_ts_sec,
       COALESCE(v.slug, '')  AS slug,
       COALESCE(v.state, '') AS state,
       COALESCE(v.city, '')  AS city
FROM chat_messages m
LEFT JOIN videos v ON v.id = m.video_id
WHERE (@platform = '' OR m.platform = @platform)
  AND (@before = 0 OR m.id < @before)
ORDER BY m.id DESC
LIMIT @n`

func chatHistory(ctx context.Context, platform string, before int64, n int) (chatHistoryPage, error) {
	var rows []struct {
		ID         int64
		Platform   string
		Username   string
		UserID     string
		MessageID  string
		Text       string
		EmittedAt  time.Time
		VideoID    *int
		VideoTsSec *float64
		Slug       string
		State      string
		City       string
	}
	err := database.GormDB().WithContext(ctx).
		Raw(chatHistorySQL, sql.Named("platform", platform), sql.Named("before", before), sql.Named("n", n)).
		Scan(&rows).Error
	if err != nil {
		return chatHistoryPage{}, err
	}
	page := chatHistoryPage{Lines: make([]chatHistoryLine, 0, len(rows))}
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		line := chatHistoryLine{
			ID: r.ID, Platform: r.Platform, Username: r.Username, UserID: r.UserID,
			MessageID: r.MessageID, Text: r.Text, EmittedAt: r.EmittedAt.UTC().Format(time.RFC3339),
			VideoID: r.VideoID, VideoTsSec: r.VideoTsSec, VideoState: r.State, VideoCity: r.City,
		}
		if r.Slug != "" {
			line.VideoFile = r.Slug + ".MP4"
		}
		page.Lines = append(page.Lines, line)
	}
	// A full page may have more before it; a short one reached the start.
	if len(rows) == n {
		page.NextBefore = rows[len(rows)-1].ID
	}
	return page, nil
}

// chatHistoryHandler serves GET /api/chat/history?before=<id>&n=&platform=.
func chatHistoryHandler(w http.ResponseWriter, r *http.Request) {
	n := queryInt(r, "n", chatHistoryDefaultLines, 1, chatHistoryMaxLines)
	before := int64(queryInt(r, "before", 0, 0, int(^uint(0)>>1)))
	page, err := chatHistory(r.Context(), r.URL.Query().Get("platform"), before, n)
	if err != nil {
		logQueryFailure(r.Context(), "chat history query failed", err, "before", before, "n", n)
		http.Error(w, `{"error":"couldn't read chat history"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(page); err != nil {
		logQueryFailure(r.Context(), "chat history encode failed", err)
	}
}
