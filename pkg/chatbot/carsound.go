package chatbot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/adanalife/tripbot/pkg/instrumentation"
	"github.com/adanalife/tripbot/pkg/obs"
	"github.com/adanalife/tripbot/pkg/users"
)

// OBS is the subset of pkg/obs the chatbot commands depend on. Tests inject a
// fake; production uses the realOBS adapter wired in New().
type OBS interface {
	SetBackgroundAudioFile(ctx context.Context, inputName, file string) error
}

// realOBS delegates to pkg/obs, which dials the OBS WebSocket per call using
// the OBS_WEBSOCKET_* env vars — the YouTube instance's point at obs-youtube.
type realOBS struct{}

func (realOBS) SetBackgroundAudioFile(ctx context.Context, inputName, file string) error {
	return obs.SetBackgroundAudioFile(ctx, inputName, file)
}

// carHumInputName is the OBS source name of the background-audio ffmpeg_source
// that !carsound repoints. Cross-repo contract: must match the source "name" in
// the OBS scene config (config/Tripbot.json.tmpl in the adanalife/obs repo).
const carHumInputName = "Car Hum"

// carSoundDir is where the OBS image's `carhum` build stage drops the rendered
// variant FLACs, as seen from inside the OBS container (the path is resolved by
// OBS, which this command drives over the WebSocket — the files are not in the
// tripbot image). Cross-repo contract: must match the COPY target in the
// adanalife/obs repo's Dockerfile{,.arm64}.
const carSoundDir = "/opt/tripbot/assets/carhum/"

// carSound is one selectable background drone. Name matches the carhum.py
// --preset and the FLAC basename (car-hum-<name>.flac).
type carSound struct {
	Name string
	Desc string
}

func (s carSound) file() string { return carSoundDir + "car-hum-" + s.Name + ".flac" }

// carSounds is the cycle order + registry of available background drones — a
// hand-maintained cross-repo contract with the adanalife/obs repo's
// carhum/render-variants.sh (one FLAC per entry) and its Dockerfiles' COPY.
// carSounds[0] is the scene's baked-in default (Tripbot.json.tmpl points
// "Car Hum" at it), so the reported "now playing" matches reality before
// anyone switches.
var carSounds = []carSound{
	{Name: "idle", Desc: "engine idling, low road"},
	{Name: "highway", Desc: "fast tarmac roar"},
	{Name: "backroad", Desc: "balanced cruise"},
	{Name: "mountain", Desc: "airy and open"},
}

// carSoundNames is the "idle, highway, …" list shown in chat.
func carSoundNames() string {
	names := make([]string, len(carSounds))
	for i, cs := range carSounds {
		names[i] = cs.Name
	}
	return strings.Join(names, ", ")
}

// findCarSound returns the index of the named sound (case-insensitive), or -1.
func findCarSound(name string) int {
	name = strings.ToLower(strings.TrimSpace(name))
	for i, cs := range carSounds {
		if cs.Name == name {
			return i
		}
	}
	return -1
}

// carSoundCmd is the public !carsound command (YouTube only — Twitch keeps the
// SomaFM source, so the "Car Hum" input doesn't exist there). With no args it
// reports what's playing, which is both the audience-facing "what's this sound"
// answer and how Dana learns which voicings viewers reach for. `next` cycles,
// `<name>` jumps to a specific one, `list` shows the options.
func (a *App) carSoundCmd(ctx context.Context, user *users.User, params []string) {
	a.carSoundMu.Lock()
	cur := a.carSoundIdx
	a.carSoundMu.Unlock()

	arg := ""
	if len(params) > 0 {
		arg = strings.ToLower(strings.TrimSpace(params[0]))
	}

	switch arg {
	case "":
		// Report current — the cheap default action.
		a.Chat.Say(fmt.Sprintf("🚗 Now playing the %q car sound (%s). Switch it: !carsound <%s>",
			carSounds[cur].Name, carSounds[cur].Desc,
			strings.ReplaceAll(carSoundNames(), ", ", "|")))
	case "list", "help", "options":
		a.Chat.Say("🚗 Car sounds: " + carSoundNames() +
			" — switch with !carsound <name>, or !carsound next to cycle")
	case "next":
		a.applyCarSound(ctx, user, (cur+1)%len(carSounds))
	default:
		idx := findCarSound(arg)
		if idx < 0 {
			a.Chat.Say(fmt.Sprintf("🚗 No car sound called %q. Options: %s", arg, carSoundNames()))
			return
		}
		a.applyCarSound(ctx, user, idx)
	}
}

// applyCarSound repoints OBS at carSounds[idx], records the selection for the
// popularity metric + logs, updates the current index, and announces in chat.
func (a *App) applyCarSound(ctx context.Context, user *users.User, idx int) {
	cs := carSounds[idx]
	if err := a.OBS.SetBackgroundAudioFile(ctx, carHumInputName, cs.file()); err != nil {
		slog.ErrorContext(ctx, "carsound switch failed",
			"err", err, "carsound", cs.Name, "username", user.Username)
		a.Chat.Say("🚗 Couldn't switch the car sound right now, try again in a bit")
		return
	}

	a.carSoundMu.Lock()
	a.carSoundIdx = idx
	a.carSoundMu.Unlock()

	// Popularity signal: a labeled counter (rank voicings in Grafana) plus a log
	// line (Loki). "carsound" is the attribute key for the voicing name.
	instrumentation.CarSoundSelections.Inc(cs.Name)
	slog.InfoContext(ctx, "carsound switched", "carsound", cs.Name, "username", user.Username)

	a.Chat.Say(fmt.Sprintf("🚗 Switched the car sound to %q — %s", cs.Name, cs.Desc))
}
