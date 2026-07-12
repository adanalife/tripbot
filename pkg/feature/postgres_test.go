package feature

import (
	"context"
	"testing"
	"time"

	"github.com/adanalife/tripbot/pkg/database/testdb"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// insertFlag writes one feature_flags row through the same struct the client
// reads back, so the GORM tags and the real column set are exercised on both
// sides. Keys used here are test-only ("test.*") — feature_flags is seeded by
// the migrations, so fixtures must not collide with the shipped flags.
func insertFlag(t *testing.T, db *gorm.DB, row flagRow) {
	t.Helper()
	if row.TargetRemovalDate.IsZero() {
		row.TargetRemovalDate = time.Now().Add(30 * 24 * time.Hour)
	}
	// The allowlist columns are NOT NULL DEFAULT '{}'; a nil pq.StringArray
	// writes NULL, not an empty array.
	if row.EnabledForUsernames == nil {
		row.EnabledForUsernames = pq.StringArray{}
	}
	if row.EnabledForRoles == nil {
		row.EnabledForRoles = pq.StringArray{}
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("insert flag %q/%q: %v", row.Key, row.Platform, err)
	}
}

// enabledInDB reads the enabled column straight from postgres, bypassing the
// client's cache — the check that a toggle actually landed on the row it
// claimed to.
func enabledInDB(t *testing.T, db *gorm.DB, key, platform string) bool {
	t.Helper()
	var row flagRow
	if err := db.Where("key = ? AND platform = ?", key, platform).First(&row).Error; err != nil {
		t.Fatalf("read back %q/%q: %v", key, platform, err)
	}
	return row.Enabled
}

func TestPostgresClient_InitialLoad(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	insertFlag(t, db, flagRow{
		Key:                 "test.ascii",
		Platform:            "twitch",
		Description:         "experimental ascii command",
		Enabled:             false,
		EnabledForUsernames: []string{"dana"},
		EnabledForRoles:     []string{"mod"},
	})

	c, err := NewPostgresClient(ctx, db, time.Minute, "twitch")
	if err != nil {
		t.Fatalf("NewPostgresClient: %v", err)
	}

	if !c.Bool(ctx, "test.ascii", EvalContext{Username: "dana"}) {
		t.Error("dana should be in the username allowlist")
	}
	if !c.Bool(ctx, "test.ascii", EvalContext{Roles: []string{"mod"}}) {
		t.Error("mod role should match the role allowlist")
	}
	if c.Bool(ctx, "test.ascii", EvalContext{Roles: []string{"regular"}}) {
		t.Error("regular user should not be enabled")
	}
	if c.Bool(ctx, "test.unknown", EvalContext{}) {
		t.Error("unknown key should evaluate to false")
	}
}

// TestPostgresClient_LoadsSeededFlags pins the client against the rows the
// migrations actually ship: a flag seeded for both platforms loads on both,
// disabled by default.
func TestPostgresClient_LoadsSeededFlags(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()

	for _, platform := range []string{"twitch", "youtube"} {
		c, err := NewPostgresClient(ctx, db, time.Minute, platform)
		if err != nil {
			t.Fatalf("NewPostgresClient(%s): %v", platform, err)
		}
		var found bool
		for _, f := range c.Snapshot(ctx) {
			if f.Key == "chatbot.weather" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: expected the seeded chatbot.weather flag in the snapshot", platform)
		}
		if c.Bool(ctx, "chatbot.weather", EvalContext{}) {
			t.Errorf("%s: chatbot.weather is seeded disabled", platform)
		}
	}
}

// TestPostgresClient_RefreshFailureRetainsCache: a failed refresh must not
// clear the last-known-good snapshot. The failure is induced with a cancelled
// context — a real query error, not a synthetic driver one.
func TestPostgresClient_RefreshFailureRetainsCache(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	insertFlag(t, db, flagRow{Key: "test.report_to_discord", Platform: "twitch", Enabled: true})

	c, err := NewPostgresClient(ctx, db, time.Minute, "twitch")
	if err != nil {
		t.Fatalf("NewPostgresClient: %v", err)
	}
	if !c.Bool(ctx, "test.report_to_discord", EvalContext{}) {
		t.Fatal("initial load: flag should be enabled")
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := c.refresh(cancelled); err == nil {
		t.Error("expected refresh to surface the query error")
	}
	if !c.Bool(ctx, "test.report_to_discord", EvalContext{}) {
		t.Error("flag should still evaluate to enabled after a failed refresh")
	}

	// Recovery: an out-of-band write is picked up by the next good refresh.
	if err := db.Model(&flagRow{}).
		Where("key = ? AND platform = ?", "test.report_to_discord", "twitch").
		Update("enabled", false).Error; err != nil {
		t.Fatalf("disable flag: %v", err)
	}
	if err := c.refresh(ctx); err != nil {
		t.Fatalf("recovery refresh: %v", err)
	}
	if c.Bool(ctx, "test.report_to_discord", EvalContext{}) {
		t.Error("flag should be disabled after a successful refresh")
	}
}

func TestPostgresClient_SetEnabled(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	insertFlag(t, db, flagRow{Key: "test.weather", Platform: "twitch", Enabled: false})

	c, err := NewPostgresClient(ctx, db, time.Minute, "twitch")
	if err != nil {
		t.Fatalf("NewPostgresClient: %v", err)
	}
	if c.Bool(ctx, "test.weather", EvalContext{}) {
		t.Fatal("flag should start disabled")
	}

	if err := c.SetEnabled(ctx, "test.weather", true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if !c.Bool(ctx, "test.weather", EvalContext{}) {
		t.Error("flag should be live-enabled immediately after SetEnabled, no poll wait")
	}
	if !enabledInDB(t, db, "test.weather", "twitch") {
		t.Error("SetEnabled should have persisted enabled=true")
	}
}

func TestPostgresClient_SetEnabledUnknownKey(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	c, err := NewPostgresClient(ctx, db, time.Minute, "twitch")
	if err != nil {
		t.Fatalf("NewPostgresClient: %v", err)
	}
	if err := c.SetEnabled(ctx, "test.missing", true); err == nil {
		t.Error("expected an error when toggling a key that doesn't exist")
	}
}

// TestPostgresClient_PlatformScoping pins the per-platform contract from
// migration 019: a client loads only its own platform's rows, and a toggle
// only touches its own platform's row — enabling a flag on youtube must not
// enable it on twitch.
func TestPostgresClient_PlatformScoping(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	insertFlag(t, db, flagRow{Key: "test.gateway", Platform: "twitch", Enabled: false})
	insertFlag(t, db, flagRow{Key: "test.gateway", Platform: "youtube", Enabled: false})
	insertFlag(t, db, flagRow{Key: "test.youtube_only", Platform: "youtube", Enabled: true})

	twitch, err := NewPostgresClient(ctx, db, time.Minute, "twitch")
	if err != nil {
		t.Fatalf("NewPostgresClient(twitch): %v", err)
	}
	youtube, err := NewPostgresClient(ctx, db, time.Minute, "youtube")
	if err != nil {
		t.Fatalf("NewPostgresClient(youtube): %v", err)
	}

	// A key that only exists on youtube is invisible (and false) on twitch.
	if twitch.Bool(ctx, "test.youtube_only", EvalContext{}) {
		t.Error("twitch client should not see a youtube-only flag")
	}
	if !youtube.Bool(ctx, "test.youtube_only", EvalContext{}) {
		t.Error("youtube client should see its own flag")
	}

	if err := youtube.SetEnabled(ctx, "test.gateway", true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if !youtube.Bool(ctx, "test.gateway", EvalContext{}) {
		t.Error("flag should be enabled on the client's own platform")
	}
	if err := twitch.refresh(ctx); err != nil {
		t.Fatalf("twitch refresh: %v", err)
	}
	if twitch.Bool(ctx, "test.gateway", EvalContext{}) {
		t.Error("toggling on youtube must not enable the flag on twitch")
	}
	if enabledInDB(t, db, "test.gateway", "twitch") {
		t.Error("the twitch row must be untouched by a youtube toggle")
	}
	if !enabledInDB(t, db, "test.gateway", "youtube") {
		t.Error("the youtube row should be enabled")
	}
}

// TestPostgresClient_SetEnabledWrongPlatform: the key exists, but not for this
// client's platform — the toggle matches zero rows and must fail loudly.
func TestPostgresClient_SetEnabledWrongPlatform(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	insertFlag(t, db, flagRow{Key: "test.twitch_only", Platform: "twitch", Enabled: false})

	youtube, err := NewPostgresClient(ctx, db, time.Minute, "youtube")
	if err != nil {
		t.Fatalf("NewPostgresClient: %v", err)
	}
	if err := youtube.SetEnabled(ctx, "test.twitch_only", true); err == nil {
		t.Error("expected an error toggling a key that has no row for this platform")
	}
	if enabledInDB(t, db, "test.twitch_only", "twitch") {
		t.Error("the twitch row must not be touched by a youtube toggle attempt")
	}
}

// TestPostgresClient_Snapshot: sorted by key, scoped to the platform, and
// carrying the targeting arrays as they round-trip through the TEXT[] columns.
func TestPostgresClient_Snapshot(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	insertFlag(t, db, flagRow{
		Key:                 "test.zzz",
		Platform:            "twitch",
		Description:         "last by key",
		EnabledForUsernames: []string{"dana", "someone_else"},
		EnabledForRoles:     []string{"mod", "vip"},
	})
	insertFlag(t, db, flagRow{Key: "test.aaa", Platform: "twitch"})
	insertFlag(t, db, flagRow{Key: "test.hidden", Platform: "youtube"})

	c, err := NewPostgresClient(ctx, db, time.Minute, "twitch")
	if err != nil {
		t.Fatalf("NewPostgresClient: %v", err)
	}

	snap := c.Snapshot(ctx)
	byKey := map[string]Flag{}
	for i, f := range snap {
		if i > 0 && snap[i-1].Key > f.Key {
			t.Errorf("snapshot not sorted by key: %q before %q", snap[i-1].Key, f.Key)
		}
		byKey[f.Key] = f
	}
	if _, ok := byKey["test.hidden"]; ok {
		t.Error("snapshot must not include another platform's rows")
	}
	if _, ok := byKey["test.aaa"]; !ok {
		t.Error("snapshot missing test.aaa")
	}
	zzz, ok := byKey["test.zzz"]
	if !ok {
		t.Fatal("snapshot missing test.zzz")
	}
	if zzz.Description != "last by key" {
		t.Errorf("description not loaded: %q", zzz.Description)
	}
	if len(zzz.EnabledForUsernames) != 2 || zzz.EnabledForUsernames[0] != "dana" {
		t.Errorf("enabled_for_usernames round-trip: %#v", zzz.EnabledForUsernames)
	}
	if len(zzz.EnabledForRoles) != 2 || zzz.EnabledForRoles[1] != "vip" {
		t.Errorf("enabled_for_roles round-trip: %#v", zzz.EnabledForRoles)
	}
	if zzz.TargetRemovalDate.IsZero() {
		t.Error("target_removal_date not loaded")
	}
}
