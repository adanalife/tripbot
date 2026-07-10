package users

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
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
	source ChatterSource
	mu     sync.Mutex
	// loggedIn maps username -> User for everyone currently in chat.
	loggedIn map[string]*User
	// lifetimeLeaderboard is the cached [username, miles] leaderboard,
	// hydrated by InitLeaderboard and rebuilt by UpdateLeaderboard.
	lifetimeLeaderboard [][]string
}

// New constructs a Sessions backed by the given ChatterSource. cmd/tripbot
// wires the production gatewayChatterSource; tests build their own.
func New(source ChatterSource) *Sessions {
	return &Sessions{
		source:   source,
		loggedIn: make(map[string]*User),
	}
}

// UpdateSession uses the chatter source to maintain the list of
// currently-logged-in users.
func (s *Sessions) UpdateSession(ctx context.Context) {
	// fetch the latest chatters from the platform
	s.source.UpdateChatters()
	currentChatters := s.source.Chatters()

	// Publish the authoritative chatter total so the admin panel's live console
	// updates the "in chat" number (and flashes it on a change) without a reload.
	eventbus.EmitViewerCount(ctx, c.Conf.Environment, c.Conf.Platform, s.source.ChatterCount())

	// Persist the same total as a viewer_samples row — the durable half of the
	// emission above, tagged with the clip currently on screen.
	viewstats.RecordSample(ctx, s.source.ChatterCount())

	// log out the people who aren't present, working from a snapshot so the
	// lock isn't held across the DB work logout does
	for username, user := range s.sessionSnapshot() {
		if _, ok := currentChatters[username]; ok {
			// they're logged in and a current chatter, do nothing
			continue
		}
		// they're logged in and NOT a current chatter, so log them out
		s.logout(ctx, user)
	}

	// log in everybody else
	//TODO: this could get slow, maybe make a list of users that need to be logged in?
	for chatter := range currentChatters {
		s.LoginIfNecessary(ctx, chatter)
	}
}

// LoginIfNecessary checks the list of currently-logged in users and will
// run login() if this user isn't currently logged in
func (s *Sessions) LoginIfNecessary(ctx context.Context, username string) *User {
	if user, ok := s.get(username); ok {
		return user
	}
	// they weren't logged in, so note in the DB
	return s.login(ctx, username)
}

// LogoutIfNecessary will log out the user if it finds them in the session
func (s *Sessions) LogoutIfNecessary(ctx context.Context, username string) {
	if user, ok := s.get(username); ok {
		s.logout(ctx, user)
	}
}

// get returns the logged-in *User for username, if present. It takes mu;
// callers must not already hold it.
func (s *Sessions) get(username string) (*User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.loggedIn[username]
	return user, ok
}

// sessionSnapshot returns a copy of the loggedIn map, so callers can iterate
// (and do slow DB work per entry) without holding mu.
func (s *Sessions) sessionSnapshot() map[string]*User {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := make(map[string]*User, len(s.loggedIn))
	maps.Copy(snapshot, s.loggedIn)
	return snapshot
}

// login will record the users presence in the DB
// TODO: do we want to make a DB update here? we could do it on logout()
func (s *Sessions) login(ctx context.Context, username string) *User {
	now := time.Now()

	user := FindOrCreate(ctx, username)
	// A zero ID means FindOrCreate couldn't get a DB row (transient Find error
	// or a failed create). Don't cache an un-saveable user in the session, or
	// every later logout tick would fail save(). Return without logging them in;
	// the next tick retries FindOrCreate and self-heals once the DB recovers.
	if user.ID == 0 {
		slog.WarnContext(ctx, "could not find or create user, skipping login", "username", username)
		return &user
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

	if err := events.Login(ctx, username, user.sessionID); err != nil {
		slog.ErrorContext(ctx, "error creating login event", "err", err)
	}

	return &user
}

// logout removes the user from the list of currently-logged in users,
// and updates the DB with their most up-to-date values
func (s *Sessions) logout(ctx context.Context, u *User) {
	sessionMiles := s.sessionMiles(ctx, *u)

	// print logout message if they're human
	if !u.IsBot {
		loggedInDur := time.Now().Sub(u.LoggedIn)
		slog.InfoContext(ctx, "logging out user",
			"user", u.String(),
			"duration", durafmt.ParseShort(loggedInDur).String(),
			"session_miles", sessionMiles,
			"monthly_miles", s.CurrentMonthlyMiles(ctx, *u),
			"guess_score", u.GetScore(ctx, scoreboards.CurrentGuessScoreboard()),
		)
	}

	// update miles
	u.Miles = s.CurrentMiles(ctx, *u)
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
	if s.IsSubscriber(*u) {
		extra += float64(s.BonusMiles(*u))
	}
	var extraMiles *float64
	if extra > 0 {
		extraMiles = &extra
	}
	if err := events.Logout(ctx, u.Username, u.sessionID, extraMiles); err != nil {
		slog.ErrorContext(ctx, "error creating logout event", "err", err)
	}

	// remove them from the session
	s.mu.Lock()
	delete(s.loggedIn, u.Username)
	s.mu.Unlock()
}

// isLoggedIn checks if the user is currently logged in
func (s *Sessions) isLoggedIn(username string) bool {
	_, ok := s.get(username)
	return ok
}

// Shutdown loops through all of the logged-in users and logs them out
func (s *Sessions) Shutdown(ctx context.Context) {
	snapshot := s.sessionSnapshot()
	if c.Conf.Verbose {
		slog.InfoContext(ctx, "logged-in users at shutdown")
		fmt.Printf("%+v\n", snapshot)
	}
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
// persists it immediately. If they're logged in, the live session copy is
// adjusted so logout doesn't clobber the correction. Returns the new total.
// The delta is deliberately NOT added to sessionExtraMiles — the caller logs a
// separate correction event carrying it, and doing both would double-count.
func (s *Sessions) CorrectMiles(ctx context.Context, username string, delta float32) float32 {
	s.mu.Lock()
	live, ok := s.loggedIn[username]
	var updated User
	if ok {
		live.Miles += delta
		updated = *live
	}
	s.mu.Unlock()
	if ok {
		updated.save(ctx)
		return updated.Miles
	}
	u := FindOrCreate(ctx, username)
	u.Miles += delta
	u.save(ctx)
	return u.Miles
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
