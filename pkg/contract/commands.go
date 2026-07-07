package contract

import (
	"bytes"
	"encoding/json"
)

// This file declares the onscreens *command* registry — the NATS subjects that
// drive the on-screen overlays (timewarp, middle text, leaderboard, GPS, flag)
// plus the passive location feed, with each payload as a JSON Schema (draft
// 2020-12). It is emitted as the sibling commands.json by
// `go generate ./pkg/contract`, exactly like eventbus.json.
//
// The distinction from eventbus.json: eventbus is *observations* tripbot
// publishes and the console consumes (facts about what happened). This is
// *commands* — imperatives that onscreens-server subscribes to and acts on.
// tripbot already publishes them (pkg/onscreens-client); this contract lets the
// admin console publish the same subjects to drive the overlays from a button,
// without hand-rebuilding the subject strings or payload shapes (the exact
// hazard the eventbus contract was created for).
//
// Source of truth: pkg/onscreens-events (the shared wire-format package, imported
// by both the tripbot publisher and the onscreens-server subscriber).
// commands_schema_test.go reflects over those structs — flattening the embedded
// Envelope — to assert the declared field names + required-ness match, so a
// struct change that isn't mirrored here fails CI. Reflection stays in the test
// so pkg/contract never imports pkg/onscreens-events (package-boundary discipline).

const commandsComment = "Generated from pkg/onscreens-events via `go generate ./pkg/contract` — do not hand-edit. The NATS onscreens-command subject + envelope registry that tripbot and the admin console publish and onscreens-server consumes; payload schemas are JSON Schema (draft 2020-12)."

// commandSubjectPattern documents the onscreens command subject convention. It
// differs from the eventbus pattern by a trailing {platform} leaf — each
// onscreens-server (onscreens-twitch / onscreens-youtube) subscribes only to its
// own platform, so a Twitch-triggered overlay never renders on YouTube.
const commandSubjectPattern = "tripbot.{env}.onscreens.{overlay}.{verb}.{platform}"

// Declared onscreens command envelope field lists. Each mirrors the json tags of
// the matching pkg/onscreens-events struct in promoted-field order (the embedded
// Envelope's emitted_at comes first). commands_schema_test.go asserts the match.
var (
	middleShowFields = []field{
		{"emitted_at", dateType(), true},
		{"msg", strType(), true},
	}
	timewarpShowFields = []field{
		{"emitted_at", dateType(), true},
		{"username", strType(), true},
	}
	leaderboardShowFields = []field{
		{"emitted_at", dateType(), true},
		{"title", strType(), true},
		{"rows", arrayType(arrayType(strType())), true},
	}
	locationUpdateFields = []field{
		{"emitted_at", dateType(), true},
		{"location", strType(), true},
		{"date", strType(), true},
	}
	// commandFields is the bare envelope shared by every hide plus gps.show —
	// the pkg/onscreens-events Command type (no data beyond the envelope).
	commandFields = []field{
		{"emitted_at", dateType(), true},
	}
)

// commandEnvelopeFields maps each declared schema title to its field list so the
// reflection test can cross-check against the real pkg/onscreens-events structs.
var commandEnvelopeFields = map[string][]field{
	"MiddleShow":      middleShowFields,
	"TimewarpShow":    timewarpShowFields,
	"LeaderboardShow": leaderboardShowFields,
	"LocationData":    locationUpdateFields,
	"Command":         commandFields,
}

// commandSubject renders one subject entry: the subject template, the transport
// (core NATS for every onscreens command — fire-and-forget, nothing replayed),
// and the payload schema.
func commandSubject(overlay, verb, schemaTitle string, fields []field) orderedObject {
	return orderedObject{
		{"subject", "tripbot.{env}.onscreens." + overlay + "." + verb + ".{platform}"},
		{"transport", "core"},
		{"schema", objectSchema(schemaTitle, fields)},
	}
}

// commandsContract builds the full onscreens command registry in stable on-disk
// order, grouped by overlay (show before hide), then the passive location feed.
func commandsContract() orderedObject {
	return orderedObject{
		{"_comment", commandsComment},
		{"subject_pattern", commandSubjectPattern},
		{"subjects", orderedObject{
			{"middle_show", commandSubject("middle", "show", "MiddleShow", middleShowFields)},
			{"middle_hide", commandSubject("middle", "hide", "Command", commandFields)},
			{"leaderboard_show", commandSubject("leaderboard", "show", "LeaderboardShow", leaderboardShowFields)},
			{"leaderboard_hide", commandSubject("leaderboard", "hide", "Command", commandFields)},
			{"timewarp_show", commandSubject("timewarp", "show", "TimewarpShow", timewarpShowFields)},
			{"timewarp_hide", commandSubject("timewarp", "hide", "Command", commandFields)},
			{"gps_show", commandSubject("gps", "show", "Command", commandFields)},
			{"gps_hide", commandSubject("gps", "hide", "Command", commandFields)},
			{"flag_hide", commandSubject("flag", "hide", "Command", commandFields)},
			{"location_update", commandSubject("location", "update", "LocationData", locationUpdateFields)},
		}},
	}
}

// MarshalCommands renders commands.json: the registry as 2-space-indented JSON
// with stable key order + trailing newline (the bytes the generator writes and
// the golden test compares against).
func MarshalCommands() ([]byte, error) {
	compact, err := commandsContract().MarshalJSON()
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, compact, "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}
