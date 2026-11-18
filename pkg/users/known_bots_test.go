package users

import (
	"context"
	"testing"

	c "github.com/adanalife/tripbot/pkg/config/tripbot"
)

type listedBots map[string]bool

func (l listedBots) IsKnownBot(name string) bool { return l[name] }

// A first login on a listed account creates it as a bot; nothing else does.
// Every other case leaves is_bot to !makebot and !unbot, which is the point:
// re-applying the list on each login would undo an operator's !unbot.
func TestFlagKnownBot(t *testing.T) {
	cfg := &c.TripbotConfig{ChannelName: "adanalife_", BotUsername: "tripbot4000"}
	list := listedBots{"commanderroot": true, "adanalife_": true, "tripbot4000": true}

	tests := []struct {
		name     string
		registry BotRegistry
		user     User
		want     bool
	}{
		{"listed, first login", list, User{Username: "commanderroot"}, true},
		{"listed, returning (an operator's !unbot stands)", list, User{Username: "commanderroot", NumVisits: 3}, false},
		{"unlisted, first login", list, User{Username: "a_person"}, false},
		{"already a bot stays one", list, User{Username: "a_person", IsBot: true}, true},
		{"the channel, whatever the list says", list, User{Username: "ADanaLife_"}, false},
		{"the bot's own account", list, User{Username: "tripbot4000"}, false},
		{"no registry wired", nil, User{Username: "commanderroot"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(cfg, nil)
			if tt.registry != nil {
				s.SetBotRegistry(tt.registry)
			}
			u := tt.user
			s.flagKnownBot(context.Background(), &u)
			if u.IsBot != tt.want {
				t.Errorf("IsBot = %v, want %v", u.IsBot, tt.want)
			}
		})
	}
}
