package users

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
	"github.com/adanalife/tripbot/pkg/eventbus"
	"github.com/adanalife/tripbot/pkg/events"
	"github.com/adanalife/tripbot/pkg/scoreboards"
	"github.com/adanalife/tripbot/pkg/viewstats"
	"github.com/google/uuid"
	"github.com/hako/durafmt"
)

// Sessions tracks the users currently logged in to one platform's chat plus
// the derived lifetime-miles leaderboard. The state lives on the struct, not
// in package-level globals, so a per-platform bot instance gets its own
// (the prerequisite for running, e.g., a YouTube bot beside the Twitch
// one). Its view of who is in chat comes from an injected ChatterSource.
//
// Sessions is accessed from multiple goroutines: the UpdateSession /
// UpdateLeaderboard crons, the inbound IRC handlers, and command dispatch.
// mu guards loggedIn and lifetimeLeaderboard; it is held only for map/slice
// access, never across DB or chatter-source calls.
type Sessions struct {
	cfg    *c.TripbotConfig
	source ChatterSource
	// chat is the optional per-tick chat-message tally drained into each
	// viewer sample. nil means samples record NULL chat_messages. Set once at
	// wiring time, before the crons start, so it needs no lock.
	chat ChatCounter
	// video is the optional airing-footage source stamped onto login/logout
	// events. nil means those events record no airing context. Set once at
	// wiring time, before the crons start, so it needs no lock.
	video VideoSource
	mu    sync.Mutex
	// loggedIn maps username -> User for everyone currently in chat.
	loggedIn map[string]*User
	// lifetimeLeaderboard is the cached [username, miles] leaderboard,
	// hydrated by InitLeaderboard and rebuilt by UpdateLeaderboard.
	lifetimeLeaderboard [][]string
}

// New constructs a Sessions backed by the given ChatterSource. cmd/tripbot
// wires the production gatewayChatterSource; tests build their own.
func New(cfg *c.TripbotConfig, source ChatterSource) *Sessions {
	return &Sessions{
		cfg:      cfg,
		source:   source,
		loggedIn: make(map[string]*User),
	}
}

// SetChatCounter installs the chat-message tally drained into each viewer
// sample. Called once during wiring, before the session crons start; leaving
// it unset records NULL chat_messages.
func (s *Sessions) SetChatCounter(counter ChatCounter) { s.chat = counter }

// chatMessages drains the wired counter for one sample tick, or reports nil
// when none is wired — NULL in the row, distinct from a silent tick's 0.
func (s *Sessions) chatMessages() *int {
	if s.chat == nil {
		return nil
	}
	n := s.chat.Drain()
	return &n
}

// SetVideoSource installs the airing-footage source for login/logout events.
// Called once during wiring, before the session crons start; leaving it unset
// records no airing context.
func (s *Sessions) SetVideoSource(v VideoSource) { s.video = v }

// currentVideoID is the clip on screen now, or 0 with no video source wired.
func (s *Sessions) currentVideoID() int {
	if s.video == nil {
		return 0
	}
	return s.video.CurrentVideoID()
}

// airing reports the footage on screen now, for stamping onto a session event.
// The zero Airing — no clip, no playhead — is what an instance with no video
// source records.
func (s *Sessions) airing() events.Airing {
	if s.video == nil {
		return events.Airing{}
	}
	secs := s.video.CurrentProgressSec()
	return events.Airing{VideoID: s.video.CurrentVideoID(), TsSec: &secs}
}

// UpdateSession uses the chatter source to maintain the list of
// currently-logged-in users.
func (s *Sessions) UpdateSession(ctx context.Context) {
	// fetch the latest chatters from the platform
	s.source.UpdateChatters()
	currentChatters := s.source.Chatters()

	// Publish the authoritative chatter total so the admin panel's live console
	// updates the "in chat" number (and flashes it on a change) without a reload.
	eventbus.EmitViewerCount(ctx, s.cfg.Environment, s.cfg.Platform, s.source.ChatterCount())

	// Refresh how many people are watching. Distinct from the chatter total
	// above: chatters are who has spoken, a self-selecting slice, so sizing
	// the audience from it understates it and biases toward whatever provokes
	// typing.
	s.source.UpdateAudience()

	// Persist both totals as a viewer_samples row — the durable half of the
	// emission above, tagged with the clip currently on screen and carrying the
	// tick's chat-message tally.
	viewstats.RecordSample(ctx, s.cfg, s.source.ChatterCount(), s.source.Audience(), s.currentVideoID(), s.chatMessages())

	// One snapshot serves both passes below, so the lock isn't held across the
	// DB work logout does and the login pass costs no per-chatter lock.
	loggedIn := s.sessionSnapshot()

	// log out the people who aren't present
	for username, user := range loggedIn {
		if _, ok := currentChatters[username]; ok {
			// they're logged in and a current chatter, do nothing
			continue
		}
		// they're logged in and NOT a current chatter, so log them out
		s.logout(ctx, user)
	}

	// log in the chatters missing from the session, so the work scales with
	// the number of arrivals rather than with the size of the audience.
	// LoginIfNecessary still re-checks under mu, covering anyone an inbound
	// message logged in since the snapshot.
	for chatter := range currentChatters {
		if _, ok := loggedIn[chatter]; ok {
			continue
		}
		s.LoginIfNecessary(ctx, chatter)
	}
}

// LoginIfNecessary checks the list of currently-logged in users and will
// run login() if this user isn't currently logged in
func (s *Sessions) LoginIfNecessary(ctx context.Context, username string) User {
	if user, ok := s.get(username); ok {
		return user
	}
	// they weren't logged in, so note in the DB
	return s.login(ctx, username)
}

// RecordPlatformUserID persists u's platform-side ID and keeps the live
// session entry in step, so later messages from the same chatter short-circuit
// on the already-set field instead of re-issuing the write every time.
func (s *Sessions) RecordPlatformUserID(ctx context.Context, u User, platformUserID string) User {
	RecordPlatformUserID(ctx, &u, platformUserID)
	s.mu.Lock()
	if live, ok := s.loggedIn[u.Username]; ok {
		live.PlatformUserID = u.PlatformUserID
	}
	s.mu.Unlock()
	return u
}

// LogoutIfNecessary will log out the user if it finds them in the session
func (s *Sessions) LogoutIfNecessary(ctx context.Context, username string) {
	if user, ok := s.get(username); ok {
		s.logout(ctx, user)
	}
}

// get returns a copy of the logged-in User for username, if present. It takes
// mu; callers must not already hold it. The copy is what makes it safe: the
// map holds *User, and handing that pointer out would let a caller read the
// entry while a miles grant writes it under mu.
func (s *Sessions) get(username string) (User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.loggedIn[username]
	if !ok {
		return User{}, false
	}
	return *user, true
}

// sessionSnapshot returns a copy of every logged-in User, so callers can
// iterate (and do slow DB work per entry) without holding mu. The entries are
// values for the reason described on get.
func (s *Sessions) sessionSnapshot() map[string]User {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := make(map[string]User, len(s.loggedIn))
	for username, user := range s.loggedIn {
		snapshot[username] = *user
	}
	return snapshot
}

// login will record the users presence in the DB
// TODO: do we want to make a DB update here? we could do it on logout()
func (s *Sessions) login(ctx context.Context, username string) User {
	now := time.Now()

	user, err := FindOrCreate(ctx, s.cfg.Platform, username)
	if err != nil {
		slog.ErrorContext(ctx, "error finding or creating user", "err", err, "username", username)
	}
	// A zero ID means FindOrCreate couldn't get a DB row (transient Find error
	// or a failed create). Don't cache an un-saveable user in the session, or
	// every later logout tick would fail save(). Return without logging them in;
	// the next tick retries FindOrCreate and self-heals once the DB recovers.
	if user.ID == 0 {
		slog.WarnContext(ctx, "could not find or create user, skipping login", "username", username)
		return user
	}
	// increment the number of visits
	user.NumVisits = user.NumVisits + 1
	// set the login time
	user.LoggedIn = now
	// assign a session ID to link this login with its eventual logout
	user.sessionID = uuid.New()
	// update the last seen date
	user.LastSeen = now
	// set their last command date yesterday
	user.lastCmd = now.AddDate(0, 0, -1)
	// set their last !location date to yesterday
	user.lastLocation = now.AddDate(0, 0, -1)
	user.save(ctx)

	// just a silly message to confirm subscriber feature is working
	if s.source.IsSubscriber(username) {
		slog.InfoContext(ctx, "subscriber logged in", "username", username)
	}

	// add them to the session
	s.mu.Lock()
	s.loggedIn[username] = &user
	s.mu.Unlock()

	if err := events.Login(ctx, s.cfg, username, user.sessionID, s.airing()); err != nil {
		slog.ErrorContext(ctx, "error creating login event", "err", err)
	}

	return user
}

// logout removes the user from the list of currently-logged in users,
// and updates the DB with their most up-to-date values
func (s *Sessions) logout(ctx context.Context, u User) {
	sessionMiles := s.sessionMiles(ctx, u)

	// print logout message if they're human
	if !u.IsBot {
		loggedInDur := time.Since(u.LoggedIn)
		slog.InfoContext(ctx, "logging out user",
			"user", u.String(),
			"duration", durafmt.ParseShort(loggedInDur).String(),
			"session_miles", sessionMiles,
			"monthly_miles", s.CurrentMonthlyMiles(ctx, u),
			"guess_score", u.GetScore(ctx, scoreboards.CurrentGuessScoreboard()),
		)
	}

	// update miles
	u.Miles = s.CurrentMiles(ctx, u)
	// update the last seen date
	u.LastSeen = time.Now()
	// store the user in the db
	u.save(ctx)

	// update the monthly scoreboard
	u.AddToScore(ctx, scoreboards.CurrentMilesScoreboard(), sessionMiles)

	// extra_miles_earned: the bonus portion of this session the events pairing
	// can't see — community sub-grants received (sessionExtraMiles) plus the 5%
	// subscriber bonus, computed the same way the live award was. Recorded at
	// source; left NULL when zero (most sessions).
	extra := float64(u.sessionExtraMiles)
	if s.IsSubscriber(u) {
		extra += float64(s.BonusMiles(u))
	}
	var extraMiles *float64
	if extra > 0 {
		extraMiles = &extra
	}
	if err := events.Logout(ctx, s.cfg, u.Username, u.sessionID, extraMiles, s.airing()); err != nil {
		slog.ErrorContext(ctx, "error creating logout event", "err", err)
	}

	// remove them from the session
	s.mu.Lock()
	delete(s.loggedIn, u.Username)
	s.mu.Unlock()
}

// CheckpointMiles banks every logged-in user's in-flight session miles — into
// users.miles and the monthly scoreboard, the same two writes logout does —
// and records what it banked so logout only counts the remainder. Without it a
// session's accrual lives only in memory until logout, so an ungraceful exit
// discards all of it and the !miles reply reads backwards. Runs on a cron, so
// a crash costs one interval instead of the whole session.
func (s *Sessions) CheckpointMiles(ctx context.Context) {
	for username := range s.sessionSnapshot() {
		user, ok := s.get(username)
		if !ok {
			continue
		}
		miles := s.sessionMiles(ctx, user)
		if miles <= 0 {
			continue
		}
		// Bank the miles first, then credit them: a failed write leaves them
		// in flight for the next tick rather than dropping them on the floor.
		// The DB work stays outside mu.
		var updated User
		s.mu.Lock()
		live, stillHere := s.loggedIn[username]
		if stillHere {
			live.Miles += miles
			live.milesCheckpointed += miles
			updated = *live
		}
		s.mu.Unlock()
		if !stillHere {
			// logged out mid-checkpoint, which banked the miles itself
			continue
		}
		updated.save(ctx)
		updated.AddToScore(ctx, scoreboards.CurrentMilesScoreboard(), miles)
	}
}

// milesCheckpointed reports how much of username's session accrual is already
// banked in the DB, or 0 for anyone not in the session. Takes mu; callers must
// not already hold it.
func (s *Sessions) milesCheckpointed(username string) float32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if live, ok := s.loggedIn[username]; ok {
		return live.milesCheckpointed
	}
	return 0.0
}

// isLoggedIn checks if the user is currently logged in
func (s *Sessions) isLoggedIn(username string) bool {
	_, ok := s.get(username)
	return ok
}

// Shutdown loops through all of the logged-in users and logs them out
func (s *Sessions) Shutdown(ctx context.Context) {
	snapshot := s.sessionSnapshot()
	for _, user := range snapshot {
		s.logout(ctx, user)
	}
}

// GiveEveryoneMiles gives all logged-in users miles
func (s *Sessions) GiveEveryoneMiles(gift float32) {
	slog.Info("giving all logged-in users gift miles", "gift", gift)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, user := range s.loggedIn {
		user.Miles += gift
		// track the grant separately so logout can record it as extra_miles_earned
		user.sessionExtraMiles += gift
	}
}

// CorrectMiles applies a manual miles delta (may be negative) to a user and
// persists it immediately, to both places a viewer sees their miles: the
// lifetime total and the current monthly scoreboard. If they're logged in, the
// live session copy is adjusted so logout doesn't clobber the correction.
// Returns the new lifetime total. The delta is deliberately NOT added to
// sessionExtraMiles — the caller logs a separate correction event carrying it,
// and doing both would double-count.
//
// Both stores move because a correction restores (or removes) miles the viewer
// should have earned by watching, and the monthly board is what !miles leads
// with and the overlay rotates. Gifted miles are the other case and stay
// lifetime-only — see GiveEveryoneMiles.
//
// An error means the correction was not applied at all: the returned total is
// meaningless, so callers must neither report it nor record a correction event
// for it. events is append-only, so an event without the matching users write
// is a permanent divergence in the rollups.
func (s *Sessions) CorrectMiles(ctx context.Context, username string, delta float32) (float32, error) {
	s.mu.Lock()
	live, ok := s.loggedIn[username]
	var updated User
	if ok {
		live.Miles += delta
		updated = *live
	}
	s.mu.Unlock()
	if ok {
		// login() never caches an ID-less user, so this row is always saveable;
		// and the live copy carries the delta either way, so logout re-persists
		// it if this write hits a transient failure.
		updated.save(ctx)
		s.correctMonthly(ctx, updated, delta)
		return updated.Miles, nil
	}
	u, err := FindOrCreate(ctx, s.cfg.Platform, username)
	if err != nil {
		// save() refuses an ID-less user, so there is no total to report: the
		// correction is dropped rather than half-applied.
		return 0, err
	}
	u.Miles += delta
	u.save(ctx)
	s.correctMonthly(ctx, u, delta)
	return u.Miles, nil
}

// correctMonthly applies a correction's delta to the current monthly
// scoreboard. A negative delta is clamped to the score on the board: miles
// clawed back may have been earned in an earlier month, and a monthly score
// below zero would render that way on the leaderboard overlay.
func (s *Sessions) correctMonthly(ctx context.Context, u User, delta float32) {
	if u.ID == 0 {
		// no DB row, so the lifetime half was dropped too — see above
		return
	}
	board := scoreboards.CurrentMilesScoreboard()
	if delta < 0 {
		owed := u.GetScore(ctx, board)
		if owed < 0 {
			// GetScore's error sentinel. Skip rather than clamp against it —
			// treating -1 as a real score would turn a clawback into a credit.
			return
		}
		if owed+delta < 0 {
			delta = -owed
		}
	}
	if delta == 0 {
		return
	}
	u.AddToScore(ctx, board, delta)
}

// The snapshot helpers below (sortedUsernameList, colorizeUsernames, humans,
// countHumans, bots, countBots) read loggedIn directly and assume the caller
// holds mu.

// sortedUsernameList creates a list of only usernames, and sort it
func (s *Sessions) sortedUsernameList() []string {
	usernames := make([]string, 0, len(s.loggedIn))
	for username := range s.loggedIn {
		usernames = append(usernames, username)
	}
	slices.Sort(usernames)
	return usernames
}

// colorizeUsernames loops over the sorted names and colorizes them
func (s *Sessions) colorizeUsernames(usernames []string) []string {
	coloredUsernames := make([]string, 0, len(usernames))
	for _, username := range usernames {
		user := *s.loggedIn[username]
		if user.IsBot {
			// don't add them to the output
			continue
		}
		// add the colored username to the list
		coloredUsernames = append(coloredUsernames, user.String())
	}
	return coloredUsernames
}

// humans returns the users in the session who are not bots
func (s *Sessions) humans() []*User {
	var humans []*User
	for _, user := range s.loggedIn {
		if !user.IsBot {
			humans = append(humans, user)
		}
	}
	return humans
}

// countHumans returns the number of humans in the session
func (s *Sessions) countHumans() int {
	return len(s.humans())
}

// bots returns the users in the session who are known bots
func (s *Sessions) bots() []*User {
	var bots []*User
	for _, user := range s.loggedIn {
		if user.IsBot {
			bots = append(bots, user)
		}
	}
	return bots
}

// countBots returns the number of bots in the session
func (s *Sessions) countBots() int {
	return len(s.bots())
}

// PrintCurrentSession simply prints info about the current session.
// ctx links the snapshot log line to the parent cron-tick span.
func (s *Sessions) PrintCurrentSession(ctx context.Context) {
	s.mu.Lock()
	coloredUsernames := s.colorizeUsernames(s.sortedUsernameList())
	humanCount := s.countHumans()
	botCount := s.countBots()
	s.mu.Unlock()

	slog.InfoContext(ctx, "session snapshot",
		"chatters", s.source.ChatterCount(),
		"humans", humanCount,
		"bots", botCount,
		"logged_in", strings.Join(coloredUsernames, ", "),
	)
}

// LoggedInCount returns the number of users currently in chat.
func (s *Sessions) LoggedInCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.loggedIn)
}
