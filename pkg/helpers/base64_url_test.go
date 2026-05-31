package helpers

import (
	"net/url"
	"testing"
)

// TestBase64SurvivesQueryParsing guards the onscreens bug (VLC-SERVER-B /
// TRIPBOT-86): the encoded value must survive being placed in an unescaped
// query parameter and parsed back out by net/url, the way onscreens-client
// builds URLs and onscreens-server reads them. Standard base64 fails this
// because its '+' is decoded to a space by query parsing; the URL-safe
// alphabet does not.
func TestBase64SurvivesQueryParsing(t *testing.T) {
	// Leaderboard overlay HTML — under standard base64 this contains a '+' at
	// byte 27, the exact payload shape behind the production errors.
	content := `<div class="lb-grid"><div class="lb-title">Correct Guesses This Month</div>`

	encoded := Base64Encode(content)

	// Mirror onscreens-client: interpolate the encoded value straight into the
	// query string with no escaping.
	u, err := url.Parse("http://vlc-server:8081/onscreens/leaderboard/show?content=" + encoded)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}

	// Mirror onscreens-server: read it back via Query().
	got, err := Base64Decode(u.Query().Get("content"))
	if err != nil {
		t.Fatalf("Base64Decode after query parse: %v", err)
	}
	if got != content {
		t.Errorf("content corrupted through query parsing: got %q, want %q", got, content)
	}
}
