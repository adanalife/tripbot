package obsEvents

import "testing"

func TestRefreshSubject(t *testing.T) {
	cases := []struct {
		env, platform, want string
	}{
		{"staging", PlatformTwitch, "tripbot.staging.obs.refresh.twitch"},
		{"prod", PlatformYouTube, "tripbot.prod.obs.refresh.youtube"},
		{"development", PlatformTwitch, "tripbot.development.obs.refresh.twitch"},
	}
	for _, c := range cases {
		if got := RefreshSubject(c.env, c.platform); got != c.want {
			t.Errorf("RefreshSubject(%q, %q) = %q, want %q", c.env, c.platform, got, c.want)
		}
	}
}
