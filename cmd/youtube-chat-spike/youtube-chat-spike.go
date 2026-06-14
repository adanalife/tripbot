// cmd/youtube-chat-spike is the de-risk spike that preceded tripbot's
// YouTube support: it proves the OAuth → active-broadcast → liveChatId →
// poll/insert loop end-to-end and measures how fast the polling cadence
// burns API quota, independent of tripbot proper.
//
// It is deliberately standalone — no tripbot/pkg imports, config via env
// vars and flags only — so running it needs nothing but an OAuth client
// and a live (unlisted is fine) broadcast. Kept as a diagnostic.
//
// Setup (one-time, manual — terraform can't create OAuth clients):
//  1. GCP console → APIs & Services → Credentials → Create OAuth client ID,
//     type "Desktop app" (Desktop clients accept any localhost redirect
//     port, so no URI registration dance).
//  2. Export YOUTUBE_CLIENT_ID + YOUTUBE_CLIENT_SECRET.
//  3. Run with -login, consent as the channel owner (live chat read/write
//     must run as the channel owner; service accounts can't do this).
//  4. Export the printed YOUTUBE_REFRESH_TOKEN.
//
// Usage:
//
//	go run ./cmd/youtube-chat-spike -login            # consent flow, prints refresh token
//	go run ./cmd/youtube-chat-spike -say "hello"      # send one chat message and exit
//	go run ./cmd/youtube-chat-spike -duration 1h      # poll live chat, then print the quota report
//
// The quota report counts requests exactly but projects daily unit burn
// from assumed per-call costs: Google's published quota table
// (developers.google.com/youtube/v3/determine_quota_cost) omits the
// liveChatMessages methods entirely, so the authoritative burn number is
// the GCP console quota graph (APIs & Services → YouTube Data API v3 →
// Quotas) after a measured run. That console reading is the Phase-B0
// verdict.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

const (
	// youtube.force-ssl is the one scope that covers both
	// liveChatMessages.list and .insert.
	scope = "https://www.googleapis.com/auth/youtube.force-ssl"

	// localhost:8090 to stay clear of tripbot's own :8080 admin server
	// when the spike runs on the same laptop.
	loginListenAddr = "localhost:8090"
	loginCallback   = "/callback"

	// Assumed per-call unit costs for the projection. NOT authoritative —
	// see the package comment; community-reported values, verify against
	// the GCP console after a run.
	assumedListCost   = 5
	assumedInsertCost = 50

	// Floor under the server-suggested polling interval, in case the API
	// ever suggests something pathological.
	minPollInterval = 2 * time.Second
)

func main() {
	login := flag.Bool("login", false, "run the OAuth consent flow and print the refresh token")
	say := flag.String("say", "", "send this message to the active broadcast's live chat and exit")
	duration := flag.Duration("duration", 15*time.Minute, "how long to poll live chat before reporting")
	flag.Parse()

	ctx := context.Background()
	conf := oauthConfig()

	if *login {
		runLogin(ctx, conf)
		return
	}

	refreshToken := os.Getenv("YOUTUBE_REFRESH_TOKEN")
	if refreshToken == "" {
		log.Fatal("YOUTUBE_REFRESH_TOKEN is not set; run with -login first")
	}
	ts := conf.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken})
	svc, err := youtube.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		log.Fatalf("building youtube service: %v", err)
	}

	counts := &callCounts{}
	chatID := discoverLiveChatID(svc, counts)

	if *say != "" {
		sendMessage(svc, counts, chatID, *say)
		counts.report(time.Duration(0))
		return
	}

	pollChat(ctx, svc, counts, chatID, *duration)
}

func oauthConfig() *oauth2.Config {
	clientID := os.Getenv("YOUTUBE_CLIENT_ID")
	clientSecret := os.Getenv("YOUTUBE_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		log.Fatal("YOUTUBE_CLIENT_ID and YOUTUBE_CLIENT_SECRET must be set (OAuth client from the GCP console)")
	}
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  "http://" + loginListenAddr + loginCallback,
		Scopes:       []string{scope},
	}
}

// runLogin walks the authorization-code flow on a localhost listener and
// prints the refresh token for the caller to export. Mirrors the shape of
// cmd/auth-bootstrap's Twitch flow, minus the DB write — pkg/youtube owns
// persistence.
func runLogin(ctx context.Context, conf *oauth2.Config) {
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		log.Fatalf("generating state: %v", err)
	}
	state := hex.EncodeToString(stateBytes)

	// AccessTypeOffline asks for a refresh token; ApprovalForce makes Google
	// re-issue one even when consent was already granted (otherwise repeat
	// logins return only an access token).
	url := conf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Println("Open this URL and consent as the channel-owner account:")
	fmt.Println()
	fmt.Println("  " + url)
	fmt.Println()

	codeCh := make(chan string, 1)
	srv := &http.Server{Addr: loginListenAddr}
	http.HandleFunc(loginCallback, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		fmt.Fprintln(w, "Got it — you can close this tab.")
		codeCh <- r.URL.Query().Get("code")
	})
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("callback listener: %v", err)
		}
	}()

	var code string
	select {
	case code = <-codeCh:
	case <-time.After(5 * time.Minute):
		log.Fatal("timed out waiting for the OAuth callback")
	}
	_ = srv.Shutdown(ctx)

	tok, err := conf.Exchange(ctx, code)
	if err != nil {
		log.Fatalf("code exchange: %v", err)
	}
	if tok.RefreshToken == "" {
		log.Fatal("no refresh token in the response (expected with AccessTypeOffline + ApprovalForce)")
	}
	fmt.Println("Success. Export this before the next run:")
	fmt.Println()
	fmt.Printf("  export YOUTUBE_REFRESH_TOKEN=%s\n", tok.RefreshToken)
}

// callCounts tracks exact request counts per method; the projection math
// happens in report.
type callCounts struct {
	broadcastsList int
	chatList       int
	chatInsert     int
	messagesSeen   int
}

func (c *callCounts) report(elapsed time.Duration) {
	fmt.Println()
	fmt.Println("--- quota report ---")
	fmt.Printf("elapsed:                  %s\n", elapsed.Round(time.Second))
	fmt.Printf("liveBroadcasts.list:      %d calls\n", c.broadcastsList)
	fmt.Printf("liveChatMessages.list:    %d calls\n", c.chatList)
	fmt.Printf("liveChatMessages.insert:  %d calls\n", c.chatInsert)
	fmt.Printf("chat messages seen:       %d\n", c.messagesSeen)
	if elapsed > 0 && c.chatList > 0 {
		avg := elapsed / time.Duration(c.chatList)
		perDay := int(float64(c.chatList) * (24 * time.Hour).Seconds() / elapsed.Seconds())
		fmt.Printf("observed poll cadence:    one list call per %s\n", avg.Round(time.Millisecond))
		fmt.Printf("projected list calls/day: %d\n", perDay)
		fmt.Printf("projected units/day:      ~%d (ASSUMING list=%d units/call — not in Google's published table)\n",
			perDay*assumedListCost, assumedListCost)
		fmt.Printf("default daily quota:      10000 units\n")
	}
	fmt.Println("authoritative burn: GCP console → APIs & Services → YouTube Data API v3 → Quotas")
}

// discoverLiveChatID finds the channel's active broadcast and returns its
// live chat ID. Note: liveBroadcasts.list treats its filters (mine,
// broadcastStatus, id) as mutually exclusive — broadcastStatus=active is
// already scoped to the authorized channel, so it's the only filter we set.
func discoverLiveChatID(svc *youtube.Service, counts *callCounts) string {
	counts.broadcastsList++
	resp, err := svc.LiveBroadcasts.List([]string{"snippet", "status"}).
		BroadcastStatus("active").
		BroadcastType("all").
		Do()
	if err != nil {
		log.Fatalf("liveBroadcasts.list: %v", err)
	}
	if len(resp.Items) == 0 {
		log.Fatal("no active broadcast found — start one (unlisted works) in YouTube Studio first")
	}
	b := resp.Items[0]
	if b.Snippet.LiveChatId == "" {
		log.Fatalf("active broadcast %q has no live chat ID (chat disabled?)", b.Snippet.Title)
	}
	// Surface privacy so it's obvious whether we're pointed at the quiet
	// test broadcast or something the audience can see.
	fmt.Printf("broadcast: %q (privacy: %s)\n", b.Snippet.Title, b.Status.PrivacyStatus)
	fmt.Printf("liveChatId: %s\n", b.Snippet.LiveChatId)
	return b.Snippet.LiveChatId
}

func sendMessage(svc *youtube.Service, counts *callCounts, chatID, text string) {
	counts.chatInsert++
	_, err := svc.LiveChatMessages.Insert([]string{"snippet"}, &youtube.LiveChatMessage{
		Snippet: &youtube.LiveChatMessageSnippet{
			LiveChatId: chatID,
			Type:       "textMessageEvent",
			TextMessageDetails: &youtube.LiveChatTextMessageDetails{
				MessageText: text,
			},
		},
	}).Do()
	if err != nil {
		log.Fatalf("liveChatMessages.insert: %v", err)
	}
	fmt.Printf("sent: %q\n", text)
}

// pollChat runs the same read loop tripbot's inbound poller uses: page
// through liveChatMessages.list at the server-suggested cadence, printing
// each message, until the duration elapses.
func pollChat(ctx context.Context, svc *youtube.Service, counts *callCounts, chatID string, duration time.Duration) {
	fmt.Printf("polling live chat for %s (Ctrl-C to stop early loses the report)...\n", duration)
	deadline := time.Now().Add(duration)
	start := time.Now()
	pageToken := ""

	for time.Now().Before(deadline) {
		counts.chatList++
		call := svc.LiveChatMessages.List(chatID, []string{"snippet", "authorDetails"})
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Context(ctx).Do()
		if err != nil {
			// Report what we measured before dying — a 403 here is itself
			// a quota data point.
			counts.report(time.Since(start))
			log.Fatalf("liveChatMessages.list: %v", err)
		}
		pageToken = resp.NextPageToken

		for _, item := range resp.Items {
			counts.messagesSeen++
			fmt.Printf("[%s] %s: %s\n",
				time.Now().Format("15:04:05"),
				item.AuthorDetails.DisplayName,
				item.Snippet.DisplayMessage)
		}

		// Respect the server-suggested interval — this is the cadence the
		// quota verdict is about.
		interval := time.Duration(resp.PollingIntervalMillis) * time.Millisecond
		if interval < minPollInterval {
			interval = minPollInterval
		}
		time.Sleep(interval)
	}
	counts.report(time.Since(start))
}
