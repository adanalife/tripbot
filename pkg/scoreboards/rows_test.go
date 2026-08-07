package scoreboards

import (
	"reflect"
	"testing"
)

// AddToScoreByName uses FirstOrCreate, so every user who has ever guessed
// keeps a row — most of them sitting at 0 early in the month. Rendering those
// would fill the leaderboard with people who haven't scored, so the zero
// filter is what makes the board mean anything.
func TestGuessRows(t *testing.T) {
	for _, tt := range []struct {
		name  string
		pairs [][]string
		want  [][]string
	}{
		{
			name:  "renders counts as whole numbers",
			pairs: [][]string{{"viewer1", "7.0"}, {"viewer2", "3.0"}},
			want:  [][]string{{"viewer1", "7"}, {"viewer2", "3"}},
		},
		{
			name:  "drops zero-scorers, keeping order",
			pairs: [][]string{{"viewer1", "5.0"}, {"viewer2", "0.0"}, {"viewer3", "2.0"}, {"viewer4", "0.0"}},
			want:  [][]string{{"viewer1", "5"}, {"viewer3", "2"}},
		},
		{
			// The start of a new month: rows exist, nobody has scored yet.
			// The caller renders this the same as an empty board.
			name:  "all-zero board reads as empty",
			pairs: [][]string{{"viewer1", "0.0"}, {"viewer2", "0.0"}},
			want:  nil,
		},
		{
			name:  "empty board stays empty",
			pairs: nil,
			want:  nil,
		},
		{
			// A malformed value shouldn't render as a blank-scored row.
			name:  "value with no decimal point survives",
			pairs: [][]string{{"viewer1", "4"}, {"viewer2", ""}},
			want:  [][]string{{"viewer1", "4"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := guessRows(tt.pairs); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("guessRows(%v) = %v, want %v", tt.pairs, got, tt.want)
			}
		})
	}
}

// Competition ranking: level scores take the better place, and the place after
// a tie skips the ones the tie used up, so a rank is always "how many are ahead
// of you, plus one".
func TestRanks(t *testing.T) {
	for _, tt := range []struct {
		name string
		rows [][]string
		want []int
	}{
		{
			name: "no ties counts up",
			rows: [][]string{{"a", "9"}, {"b", "5"}, {"c", "2"}},
			want: []int{1, 2, 3},
		},
		{
			name: "a tie shares the better place and the next one skips",
			rows: [][]string{{"a", "9"}, {"b", "9"}, {"c", "2"}},
			want: []int{1, 1, 3},
		},
		{
			name: "a tie further down leaves the places above it alone",
			rows: [][]string{{"a", "9"}, {"b", "5"}, {"c", "5"}, {"d", "1"}},
			want: []int{1, 2, 2, 4},
		},
		{
			// Everyone level is everyone first — there is nobody ahead.
			name: "every row tied",
			rows: [][]string{{"a", "3"}, {"b", "3"}, {"c", "3"}},
			want: []int{1, 1, 1},
		},
		{
			// Equal scores are only a tie when they touch; the board is sorted,
			// so this shape means the sort was wrong, not that a and c tie.
			name: "equal scores split apart do not share a place",
			rows: [][]string{{"a", "9"}, {"b", "5"}, {"c", "9"}},
			want: []int{1, 2, 3},
		},
		{
			name: "empty board",
			rows: nil,
			want: []int{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := Ranks(tt.rows); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Ranks() = %v, want %v", got, tt.want)
			}
		})
	}
}
