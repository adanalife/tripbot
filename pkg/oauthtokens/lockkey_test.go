package oauthtokens

import "testing"

// TestLockKeyParityWithGateway pins the advisory-lock key derivation to golden
// values. platform-gateway's internal/tokenstore.lockKey pins the same pairs
// and MUST stay identical: the two services coordinate a token refresh only by
// taking the same Postgres advisory lock, so a key that drifts on either side
// breaks that silently — both would refresh concurrently, each believing it
// holds the lock, and every other test here would stay green.
func TestLockKeyParityWithGateway(t *testing.T) {
	cases := map[string]int64{
		"tripbot4000": 446341586975914733,
		"adanalife_":  -4978836094196985472,
	}
	for identity, want := range cases {
		if got := lockKey("twitch", identity); got != want {
			t.Errorf("lockKey(twitch, %q) = %d, want %d (must match platform-gateway's internal/tokenstore.lockKey)", identity, got, want)
		}
	}
}
