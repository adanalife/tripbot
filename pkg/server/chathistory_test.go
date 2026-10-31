package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/database/testdb"
	"github.com/adanalife/tripbot/pkg/events"
)

// The empty-data contract: lines is [], never null, and the cursor is 0.
func TestChatHistoryHandler_EmptyDataShape(t *testing.T) {
	testdb.New(t)
	rec := insightsGET(t, "/api/chat/history", chatHistoryHandler)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); body != "{\"lines\":[],\"next_before\":0}\n" {
		t.Errorf("body = %s", body)
	}
}

// Pages read oldest-first, the cursor names the page before, a short page
// ends the walk, and a stamped line carries its clip's file and state.
func TestChatHistoryHandler_PagesBack(t *testing.T) {
	db := testdb.New(t)
	if err := db.Exec(`INSERT INTO videos (id, slug, state, lat, lng) VALUES (7, '2018_0514_224801_013', 'Utah', 0, 0)`).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &c.TripbotConfig{Platform: "twitch"}
	secs := 83.9
	for i, text := range []string{"one", "two", "three"} {
		l := events.ChatLine{Username: "alice", Text: text}
		if i == 2 {
			l.VideoID, l.TsSec = 7, &secs
		}
		if err := events.RecordChat(context.Background(), cfg, l); err != nil {
			t.Fatal(err)
		}
	}
	var page chatHistoryPage
	rec := insightsGET(t, "/api/chat/history?n=2", chatHistoryHandler)
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Lines) != 2 || page.Lines[0].Text != "two" || page.Lines[1].Text != "three" || page.NextBefore == 0 {
		t.Fatalf("newest page = %+v", page)
	}
	if got := page.Lines[1]; got.VideoFile != "2018_0514_224801_013.MP4" || got.VideoState != "Utah" || got.VideoTsSec == nil || *got.VideoTsSec != 83.9 {
		t.Errorf("stamped line = %+v", got)
	}
	rec = insightsGET(t, "/api/chat/history?n=2&before="+strconv.FormatInt(page.NextBefore, 10), chatHistoryHandler)
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Lines) != 1 || page.Lines[0].Text != "one" || page.NextBefore != 0 {
		t.Fatalf("older page = %+v", page)
	}
	rec = insightsGET(t, "/api/chat/history?platform=youtube", chatHistoryHandler)
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Lines) != 0 {
		t.Errorf("youtube page = %+v, want none", page)
	}
}

// `until` jumps to the end of a UTC day: lines after it drop out, the day
// itself stays, and the cursor still pages back from there.
func TestChatHistoryHandler_Until(t *testing.T) {
	db := testdb.New(t)
	cfg := &c.TripbotConfig{Platform: "twitch"}
	for _, text := range []string{"before", "on the day", "after"} {
		if err := events.RecordChat(context.Background(), cfg, events.ChatLine{Username: "alice", Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	for text, at := range map[string]string{"before": "2026-10-01 12:00Z", "on the day": "2026-10-02 23:59Z", "after": "2026-10-03 00:00Z"} {
		if err := db.Exec(`UPDATE chat_messages SET emitted_at = ? WHERE text = ?`, at, text).Error; err != nil {
			t.Fatal(err)
		}
	}
	var page chatHistoryPage
	rec := insightsGET(t, "/api/chat/history?n=1&until=2026-10-02", chatHistoryHandler)
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Lines) != 1 || page.Lines[0].Text != "on the day" || page.NextBefore == 0 {
		t.Fatalf("until page = %+v", page)
	}
	rec = insightsGET(t, "/api/chat/history?until=nonsense", chatHistoryHandler)
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Lines) != 3 {
		t.Errorf("a bad until filtered: %+v", page)
	}
}

// A line the platform removed comes back flagged deleted, so the console
// strikes it; its neighbors don't.
func TestChatHistoryHandler_StruckLine(t *testing.T) {
	testdb.New(t)
	ctx := context.Background()
	cfg := &c.TripbotConfig{Platform: "twitch"}
	for _, id := range []string{"m1", "m2"} {
		if err := events.RecordChat(ctx, cfg, events.ChatLine{Username: "alice", UserID: "1", MessageID: id, Text: id}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := events.StrikeChat(ctx, cfg, "1", "m1", time.Now()); err != nil {
		t.Fatal(err)
	}
	var page chatHistoryPage
	rec := insightsGET(t, "/api/chat/history", chatHistoryHandler)
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Lines) != 2 || !page.Lines[0].Deleted || page.Lines[1].Deleted {
		t.Fatalf("lines = %+v, want m1 deleted and m2 not", page.Lines)
	}
}
