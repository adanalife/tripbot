package main

import (
	crand "crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	defaultLog := filepath.Join("logs", fmt.Sprintf("streamtest-%s.log", time.Now().Format("2006-01-02")))
	logPath := flag.String("log", defaultLog, "path to JSON-lines session log")
	timeoutSec := flag.Int("timeout", 5, "seconds to wait for bot reply per command")
	clientID := flag.String("client-id", os.Getenv("TWITCH_CLIENT_ID"), "Twitch app client ID (or set TWITCH_CLIENT_ID)")
	channel := flag.String("channel", envOr("CHANNEL_NAME", "adanalife_staging"), "channel to send commands in")
	botUser := flag.String("bot", envOr("BOT_USERNAME", "tripbot4000"), "bot username whose replies to capture")
	tokenPath := flag.String("token-cache", ".token.json", "where to cache the OAuth token")
	flag.Parse()

	if *clientID == "" {
		fmt.Fprintln(os.Stderr, "missing -client-id (or TWITCH_CLIENT_ID env). Register one at https://dev.twitch.tv/console/apps.")
		os.Exit(1)
	}

	auth, err := Authorize(*clientID, *tokenPath)
	if err != nil {
		log.Fatalf("twitch oauth: %v", err)
	}

	prior, err := LoadResults(*logPath)
	if err != nil {
		log.Fatalf("read existing log %s: %v", *logPath, err)
	}
	sessionLog, err := OpenLog(*logPath)
	if err != nil {
		log.Fatalf("open log %s: %v", *logPath, err)
	}
	defer sessionLog.Close()

	runner := NewRunner(auth.Login, auth.IRCToken(), *channel, *botUser, time.Duration(*timeoutSec)*time.Second)
	if err := runner.Connect(15 * time.Second); err != nil {
		log.Fatalf("twitch IRC connect: %v", err)
	}
	defer runner.Close()

	model := NewModel(runner, sessionLog, prior, *channel, *botUser, auth.Login, newSessionID(), buildVersion())
	if _, err := tea.NewProgram(model, tea.WithAltScreen()).Run(); err != nil {
		log.Fatalf("tui: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newSessionID() string {
	var b [8]byte
	if _, err := crand.Read(b[:]); err != nil {
		return fmt.Sprintf("ts%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var rev, mod string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			mod = s.Value
		}
	}
	if rev == "" {
		return ""
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if mod == "true" {
		rev += "-dirty"
	}
	return rev
}
