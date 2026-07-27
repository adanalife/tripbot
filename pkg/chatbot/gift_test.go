package chatbot

import "testing"

// The ladder must resolve every value, including one below the lowest rung —
// a gift is a gift, and a gift that silently did nothing would read as broken
// to the viewer who paid for it.
func TestEffectForResolvesEveryValue(t *testing.T) {
	for _, value := range []int{-1, 0, 1, 9, 10, 99, 100, 1_000_000} {
		if effectFor(value) == "" {
			t.Errorf("value %d resolved to no effect", value)
		}
	}
}

// effectFor takes the highest rung a gift reaches. Asserted against a local
// ladder rather than the production one so the property still holds after
// giftTiers is retuned (every rung is a timewarp today, which would make an
// assertion against the real table vacuous).
func TestEffectForTakesHighestRungReached(t *testing.T) {
	prev := giftTiers
	t.Cleanup(func() { giftTiers = prev })
	giftTiers = []giftTier{
		{MinValue: 0, Effect: "small"},
		{MinValue: 10, Effect: "medium"},
		{MinValue: 100, Effect: "large"},
	}

	cases := map[int]giftEffect{
		0: "small", 1: "small", 9: "small",
		10: "medium", 99: "medium",
		100: "large", 5000: "large",
	}
	for value, want := range cases {
		if got := effectFor(value); got != want {
			t.Errorf("effectFor(%d) = %q, want %q", value, got, want)
		}
	}
}

// The rungs must stay sorted ascending — effectFor walks the table in order and
// stops at the first rung above the gift's value, so an out-of-order rung would
// be unreachable.
func TestGiftTiersAreSortedAscending(t *testing.T) {
	if len(giftTiers) == 0 {
		t.Fatal("giftTiers is empty; effectFor indexes rung 0 unconditionally")
	}
	if giftTiers[0].MinValue != 0 {
		t.Errorf("first rung MinValue = %d, want 0 so every gift resolves", giftTiers[0].MinValue)
	}
	for i := 1; i < len(giftTiers); i++ {
		if giftTiers[i].MinValue <= giftTiers[i-1].MinValue {
			t.Errorf("rung %d (MinValue %d) does not exceed rung %d (MinValue %d)",
				i, giftTiers[i].MinValue, i-1, giftTiers[i-1].MinValue)
		}
	}
}
