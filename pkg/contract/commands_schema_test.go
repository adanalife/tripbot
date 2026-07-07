package contract

import (
	"reflect"
	"strings"
	"testing"

	oe "github.com/adanalife/tripbot/pkg/onscreens-events"
)

// TestCommandsSchemaMatchesStructs reflects over the real pkg/onscreens-events
// command envelope structs and asserts the declared field lists (name +
// required) match field-for-field, in promoted order. Unlike the eventbus
// structs these embed a shared Envelope, so the walk flattens anonymous struct
// fields — the embedded Envelope's emitted_at is promoted to the front, exactly
// as it lands on the wire. A rename, added/removed field, or changed omitempty
// that isn't mirrored in commands.go fails here, so commands.json (and the
// Python types generated from it) can't silently drift from the wire format.
//
// Reflection lives in this test, not in commands.go, so pkg/contract never
// imports pkg/onscreens-events (package-boundary discipline).
func TestCommandsSchemaMatchesStructs(t *testing.T) {
	structs := map[string]reflect.Type{
		"MiddleShow":      reflect.TypeOf(oe.MiddleShow{}),
		"TimewarpShow":    reflect.TypeOf(oe.TimewarpShow{}),
		"LeaderboardShow": reflect.TypeOf(oe.LeaderboardShow{}),
		"LocationData":    reflect.TypeOf(oe.LocationData{}),
		"Command":         reflect.TypeOf(oe.Command{}),
	}

	type nameReq struct {
		name     string
		required bool
	}

	for title, typ := range structs {
		declared, ok := commandEnvelopeFields[title]
		if !ok {
			t.Errorf("%s: no declared field list in commandEnvelopeFields", title)
			continue
		}

		fromStruct := flattenJSONFields(typ)
		if len(fromStruct) != len(declared) {
			t.Errorf("%s: struct has %d json fields, declared schema has %d",
				title, len(fromStruct), len(declared))
			continue
		}
		for i := range fromStruct {
			if fromStruct[i].name != declared[i].Name {
				t.Errorf("%s field %d: struct json name %q != declared %q",
					title, i, fromStruct[i].name, declared[i].Name)
			}
			if fromStruct[i].required != declared[i].Required {
				t.Errorf("%s field %q: struct required=%v != declared required=%v",
					title, fromStruct[i].name, fromStruct[i].required, declared[i].Required)
			}
		}
	}
}

type jsonField struct {
	name     string
	required bool
}

// flattenJSONFields walks a struct's json-tagged fields in promotion order,
// recursing into anonymous embedded structs (so the embedded Envelope's
// emitted_at appears where Go promotes it — at the front). Mirrors how
// encoding/json flattens the embedded envelope onto the wire.
func flattenJSONFields(typ reflect.Type) []jsonField {
	var out []jsonField
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Anonymous && f.Type.Kind() == reflect.Struct && f.Tag.Get("json") == "" {
			out = append(out, flattenJSONFields(f.Type)...)
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		parts := strings.Split(tag, ",")
		required := true
		for _, p := range parts[1:] {
			if p == "omitempty" {
				required = false
			}
		}
		out = append(out, jsonField{name: parts[0], required: required})
	}
	return out
}
