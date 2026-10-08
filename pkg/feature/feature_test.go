package feature

import (
	"context"
	"testing"
)

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name string
		flag Flag
		ctx  EvalContext
		want bool
	}{
		{
			name: "global default off, no targeting match",
			flag: Flag{Enabled: false},
			ctx:  EvalContext{Username: "dana", Roles: []string{"regular"}},
			want: false,
		},
		{
			name: "global default on, no targeting match",
			flag: Flag{Enabled: true},
			ctx:  EvalContext{Username: "dana", Roles: []string{"regular"}},
			want: true,
		},
		{
			name: "username allowlist beats global off",
			flag: Flag{Enabled: false, EnabledForUsernames: []string{"dana"}},
			ctx:  EvalContext{Username: "dana"},
			want: true,
		},
		{
			name: "role allowlist beats global off",
			flag: Flag{Enabled: false, EnabledForRoles: []string{"mod"}},
			ctx:  EvalContext{Username: "dana", Roles: []string{"mod", "sub"}},
			want: true,
		},
		{
			name: "username allowlist no-match falls through to role allowlist",
			flag: Flag{
				EnabledForUsernames: []string{"someoneElse"},
				EnabledForRoles:     []string{"mod"},
			},
			ctx:  EvalContext{Username: "dana", Roles: []string{"mod"}},
			want: true,
		},
		{
			name: "empty username never matches username allowlist",
			flag: Flag{EnabledForUsernames: []string{"", "dana"}},
			ctx:  EvalContext{Username: ""},
			want: false,
		},
		{
			name: "role allowlist requires intersection",
			flag: Flag{EnabledForRoles: []string{"mod"}},
			ctx:  EvalContext{Roles: []string{"sub", "regular"}},
			want: false,
		},
		{
			name: "system-level eval (no user) honors global default",
			flag: Flag{Enabled: true},
			ctx:  EvalContext{Env: "prod-1"},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := evaluate(tc.flag, tc.ctx); got != tc.want {
				t.Errorf("evaluate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestInMemoryClient_UnknownKeyReturnsFalse(t *testing.T) {
	c := NewInMemoryClient(map[string]Flag{
		"chatbot.known": {Enabled: true},
	})
	if c.Bool(context.Background(), "chatbot.unknown", EvalContext{}) {
		t.Error("unknown key should evaluate to false")
	}
}

func TestInMemoryClient_NilMap(t *testing.T) {
	c := NewInMemoryClient(nil)
	if c.Bool(context.Background(), "chatbot.anything", EvalContext{}) {
		t.Error("nil-map client should evaluate every key to false")
	}
}

func TestInMemoryClient_DelegatesEvaluation(t *testing.T) {
	c := NewInMemoryClient(map[string]Flag{
		"chatbot.ascii": {
			EnabledForRoles: []string{"mod"},
		},
	})
	ctx := context.Background()
	if !c.Bool(ctx, "chatbot.ascii", EvalContext{Roles: []string{"mod"}}) {
		t.Error("mod role should enable the flag")
	}
	if c.Bool(ctx, "chatbot.ascii", EvalContext{Roles: []string{"regular"}}) {
		t.Error("regular role should leave the flag off")
	}
}
