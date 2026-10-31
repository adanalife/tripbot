package rotator

import "unicode/utf8"

// Budget describes how much room a corner's text actually has on the OBS canvas.
// The two corners are not the same size — the right corner's grey-box underlay
// is 369px against the left's 564px — so the same line can sit comfortably on
// the left and overflow on the right.
//
// onscreens-server's browser source renders text at FontSizePx and steps down 1px
// at a time until the line fits FitWidthPx, then wraps to a second line if it
// hits MinFontSizePx and still doesn't fit. So a line "fits" if it fits at
// MinFontSizePx; past that it doesn't get clipped, it gets small and then wraps.
// That's what the console warns about.
//
// FontFamilyCSS is the CSS font stack the overlay renders in, exported so the
// console can measure a candidate line in the *same* font via canvas
// measureText instead of guessing.
type Budget struct {
	Side          Side   `json:"side"`
	FitWidthPx    int    `json:"fit_width_px"`
	FontSizePx    int    `json:"font_size_px"`
	MinFontSizePx int    `json:"min_font_size_px"`
	FontFamilyCSS string `json:"font_family_css"`
}

// FontFamilyCSS is the corner rotators' CSS font stack. onscreens-server's
// onscreen registry renders both corners in it; the console measures against it.
const FontFamilyCSS = `"Trebuchet MS", sans-serif`

// Corner render budgets. These are the numbers onscreens-server's onscreen
// registry renders with — it reads them from here so the console's warning and
// the actual overlay layout can't drift apart.
var budgets = map[Side]Budget{
	SideLeft: {
		Side: SideLeft, FitWidthPx: 564, FontSizePx: 28, MinFontSizePx: 18,
		FontFamilyCSS: FontFamilyCSS,
	},
	SideRight: {
		Side: SideRight, FitWidthPx: 369, FontSizePx: 28, MinFontSizePx: 18,
		FontFamilyCSS: FontFamilyCSS,
	},
}

// BudgetFor returns the render budget for a corner (SideRight for anything but
// SideLeft, matching Config.Corner).
func BudgetFor(side Side) Budget {
	if side == SideLeft {
		return budgets[SideLeft]
	}
	return budgets[SideRight]
}

// Budgets returns both corner budgets, for handing to the console in one shot.
func Budgets() []Budget { return []Budget{budgets[SideLeft], budgets[SideRight]} }

// avgAdvanceEms is the mean glyph advance of mixed-case Trebuchet MS text, as a
// fraction of the font size. It backs the server-side length estimate only — the
// console does the exact measurement in the browser, where the real font metrics
// live. Measured across the shipped default lines; deliberately a touch
// generous, since the estimate's job is rejecting absurd input, not drawing the
// warning threshold.
const avgAdvanceEms = 0.5

// EstimatedMaxRunes is roughly how many characters fit on one line at the
// shrink-to-fit floor. Backticks around a `!command` render as a 0.9em monospace
// pill with a little padding, which lands near enough to the plain-text advance
// that the estimate holds either way.
func (b Budget) EstimatedMaxRunes() int {
	if b.MinFontSizePx <= 0 {
		return 0
	}
	return int(float64(b.FitWidthPx) / (float64(b.MinFontSizePx) * avgAdvanceEms))
}

// HardMaxRunes is the server-side rejection threshold: generous enough that the
// console's exact per-line warning stays the real UX, tight enough that a
// paste-gone-wrong can't push a paragraph onto the overlay. Text between the
// estimate and this limit is accepted — it renders, just small or on two lines,
// and that's the author's call to make.
func (b Budget) HardMaxRunes() int { return b.EstimatedMaxRunes() * 3 }

// TooLong reports whether text blows past the hard limit for this corner.
func (b Budget) TooLong(text string) bool {
	return utf8.RuneCountInString(text) > b.HardMaxRunes()
}
