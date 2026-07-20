package contract

import (
	"reflect"
	"strings"
	"testing"

	"github.com/adanalife/tripbot/pkg/eventbus"
)

// TestEventbusSchemaMatchesStructs reflects over the real pkg/eventbus envelope
// structs and asserts the declared field lists (name + required) match field-
// for-field, in order. A struct rename, added/removed field, or changed
// omitempty that isn't mirrored in eventbus.go fails here — so the generated
// eventbus.json (and the Python types generated from it) can't silently drift
// from the actual wire format.
//
// Reflection lives in this test, not in eventbus.go, so pkg/contract itself
// never imports pkg/eventbus (package-boundary discipline). It only inspects
// json names + omitempty, not the JSON Schema node types — name/required drift
// is the failure mode that has actually bitten; deep type-shape checking would
// duplicate the schema for little extra safety.
func TestEventbusSchemaMatchesStructs(t *testing.T) {
	structs := map[string]reflect.Type{
		"ChatMessage":       reflect.TypeOf(eventbus.ChatMessage{}),
		"ViewerCount":       reflect.TypeOf(eventbus.ViewerCount{}),
		"VideoChanged":      reflect.TypeOf(eventbus.VideoChanged{}),
		"AuthStatus":        reflect.TypeOf(eventbus.AuthStatus{}),
		"AuthAccount":       reflect.TypeOf(eventbus.AuthAccount{}),
		"YoutubeBroadcast":  reflect.TypeOf(eventbus.YoutubeBroadcast{}),
		"FacebookBroadcast": reflect.TypeOf(eventbus.FacebookBroadcast{}),
	}

	type nameReq struct {
		name     string
		required bool
	}

	for title, typ := range structs {
		declared, ok := envelopeFields[title]
		if !ok {
			t.Errorf("%s: no declared field list in envelopeFields", title)
			continue
		}

		var fromStruct []nameReq
		for i := 0; i < typ.NumField(); i++ {
			tag := typ.Field(i).Tag.Get("json")
			if tag == "" || tag == "-" {
				continue
			}
			parts := strings.Split(tag, ",")
			name := parts[0]
			required := true
			for _, p := range parts[1:] {
				if p == "omitempty" {
					required = false
				}
			}
			fromStruct = append(fromStruct, nameReq{name: name, required: required})
		}

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
