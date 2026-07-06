package contract

import (
	"bytes"
	"encoding/json"
)

// This file declares the eventbus registry — the NATS subjects tripbot
// publishes and tripbot-console consumes, with each payload as a JSON Schema
// (draft 2020-12). It is emitted as the sibling eventbus.json by
// `go generate ./pkg/contract`, exactly like the service/port contract.
//
// Why it lives here: tripbot's pkg/eventbus is the single source of truth for
// these subjects + envelopes (the only publisher), but the Python console
// rebuilt the subject strings + payload field names by hand, with no shared
// artifact — a Go rename silently broke the consumer (and stalled real work,
// e.g. a wrong subject string is an on-stream no-op). eventbus.json is the one
// synced file the console reads (`task contract:sync`), so every subject,
// transport, stream, and exact payload shape is discoverable in one place
// instead of guessed.
//
// Drift guard: this is hand-declared (like the service/port constants), and
// eventbus_schema_test.go reflects over the real pkg/eventbus structs to assert
// the declared field names + required-ness match field-for-field — so a struct
// change that isn't mirrored here fails CI. Reflection stays in the test so
// pkg/contract itself never imports pkg/eventbus (package-boundary discipline).
//
// JSON Schema (not a bespoke shape) so a consumer can run a standard
// schema->types generator, and so a later protobuf migration maps 1-1: each
// schema title is the future message name, and the snake_case fields already
// match protobuf convention.

const eventbusComment = "Generated from pkg/eventbus via `go generate ./pkg/contract` — do not hand-edit. The NATS subject + envelope registry tripbot publishes and tripbot-console consumes; payload schemas are JSON Schema (draft 2020-12)."

// subjectPattern documents the project-wide subject convention. The env segment
// is templated per deploy; {domain}.{event} is filled by each subject below.
const subjectPattern = "tripbot.{env}.{domain}.{event}"

// orderedObject marshals as a JSON object with members in slice order, so the
// generated eventbus.json is stable + reviewable (mirrors the ordered Marshal
// the service/port contract uses). Reuses pair for its entries.
type orderedObject []pair

func (o orderedObject) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, e := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := marshalNoEscape(e.Key)
		if err != nil {
			return nil, err
		}
		val, err := marshalNoEscape(e.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// marshalNoEscape JSON-marshals v without HTML escaping (so subject templates
// like "tripbot.{env}.{domain}.{event}" render literally rather than as <
// sequences) and without the trailing newline json.Encoder appends.
func marshalNoEscape(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(b.Bytes(), "\n"), nil
}

// field is one declared envelope property: its JSON name, its JSON Schema node,
// and whether it's required (present unconditionally — i.e. the Go struct field
// carries no omitempty). The field lists below are the source the reflection
// test checks against the real pkg/eventbus structs.
type field struct {
	Name     string
	Schema   orderedObject
	Required bool
}

// JSON Schema node builders.
func strType() orderedObject  { return orderedObject{{"type", "string"}} }
func dateType() orderedObject { return orderedObject{{"type", "string"}, {"format", "date-time"}} }
func intType() orderedObject  { return orderedObject{{"type", "integer"}} }
func numType() orderedObject  { return orderedObject{{"type", "number"}} }
func boolType() orderedObject { return orderedObject{{"type", "boolean"}} }

func refType(name string) orderedObject { return orderedObject{{"$ref", "#/$defs/" + name}} }

func arrayType(items orderedObject) orderedObject {
	return orderedObject{{"type", "array"}, {"items", items}}
}

// objectSchema renders []field as a JSON Schema object node (type/title/
// properties/required), properties in declared order.
func objectSchema(title string, fields []field) orderedObject {
	props := make(orderedObject, 0, len(fields))
	required := make([]string, 0, len(fields))
	for _, f := range fields {
		props = append(props, pair{f.Name, f.Schema})
		if f.Required {
			required = append(required, f.Name)
		}
	}
	return orderedObject{
		{"type", "object"},
		{"title", title},
		{"properties", props},
		{"required", required},
	}
}

// Declared envelope field lists. Each mirrors the json tags (name + omitempty)
// of the matching pkg/eventbus struct, in struct-declaration order;
// eventbus_schema_test.go asserts the match.
var (
	chatMessageFields = []field{
		{"platform", strType(), false},
		{"username", strType(), true},
		{"text", strType(), true},
		{"emitted_at", dateType(), true},
	}
	viewerCountFields = []field{
		{"platform", strType(), false},
		{"count", intType(), true},
		{"emitted_at", dateType(), true},
	}
	videoChangedFields = []field{
		{"platform", strType(), false},
		{"file", strType(), true},
		{"state", strType(), true},
		{"flagged", boolType(), true},
		{"lat", numType(), true},
		{"lng", numType(), true},
		{"emitted_at", dateType(), true},
	}
	authStatusFields = []field{
		{"platform", strType(), true},
		{"accounts", arrayType(refType("AuthAccount")), true},
		{"emitted_at", dateType(), true},
	}
	authAccountFields = []field{
		{"account", strType(), true},
		{"login_as", strType(), false},
		{"expires_at", dateType(), false},
		{"reason", strType(), false},
	}
	youtubeBroadcastFields = []field{
		{"video_id", strType(), true},
		{"live", boolType(), true},
		{"privacy", strType(), true},
		{"emitted_at", dateType(), true},
	}
)

// envelopeFields maps each declared schema title to its field list so the
// reflection test can cross-check against the real pkg/eventbus structs.
var envelopeFields = map[string][]field{
	"ChatMessage":      chatMessageFields,
	"ViewerCount":      viewerCountFields,
	"VideoChanged":     videoChangedFields,
	"AuthStatus":       authStatusFields,
	"AuthAccount":      authAccountFields,
	"YoutubeBroadcast": youtubeBroadcastFields,
}

// eventbusContract builds the full registry in stable on-disk order: the four
// observation subjects, then the shared $defs. Order within each subject is
// subject -> wildcard? -> transport -> stream? -> schema.
func eventbusContract() orderedObject {
	return orderedObject{
		{"_comment", eventbusComment},
		{"subject_pattern", subjectPattern},
		{"subjects", orderedObject{
			{"chat_message", orderedObject{
				{"subject", "tripbot.{env}.chat.message"},
				{"transport", "jetstream"},
				{"stream", "TRIPBOT_CHAT"},
				{"schema", objectSchema("ChatMessage", chatMessageFields)},
			}},
			{"viewers_count", orderedObject{
				{"subject", "tripbot.{env}.viewers.count"},
				{"transport", "core"},
				{"schema", objectSchema("ViewerCount", viewerCountFields)},
			}},
			{"video_changed", orderedObject{
				{"subject", "tripbot.{env}.video.changed"},
				{"transport", "jetstream"},
				{"stream", "TRIPBOT_VIDEO"},
				{"schema", objectSchema("VideoChanged", videoChangedFields)},
			}},
			{"auth_status", orderedObject{
				{"subject", "tripbot.{env}.auth.status.{platform}"},
				{"wildcard", "tripbot.{env}.auth.status.*"},
				{"transport", "jetstream"},
				{"stream", "TRIPBOT_AUTH"},
				{"schema", objectSchema("AuthStatus", authStatusFields)},
			}},
			{"youtube_broadcast", orderedObject{
				{"subject", "tripbot.{env}.youtube.broadcast"},
				{"transport", "jetstream"},
				{"stream", "TRIPBOT_YOUTUBE"},
				{"schema", objectSchema("YoutubeBroadcast", youtubeBroadcastFields)},
			}},
		}},
		{"$defs", orderedObject{
			{"AuthAccount", objectSchema("AuthAccount", authAccountFields)},
		}},
	}
}

// MarshalEventbus renders eventbus.json: the registry as 2-space-indented JSON
// with stable key order + trailing newline (the bytes the generator writes and
// the golden test compares against).
func MarshalEventbus() ([]byte, error) {
	compact, err := eventbusContract().MarshalJSON()
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
