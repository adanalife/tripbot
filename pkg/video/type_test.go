package video

import (
	"testing"
	"time"
)

func TestSlug(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"path with .MP4", "/some/dir/2018_0514_224801_013.MP4", "2018_0514_224801_013"},
		{"bare filename", "2018_0514_224801_013.MP4", "2018_0514_224801_013"},
		{"lowercase ext", "video.mp4", "video"},
		{"no extension", "noextfile", "noextfile"},
		{"multi-dot keeps all but last segment", "my.video.file.MP4", "my.video.file"},
		{"trailing dot", "trailing.", "trailing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slug(tt.in)
			if got != tt.want {
				t.Fatalf("slug(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRemoveFileExtension(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"foo.txt", "foo"},
		{"archive.tar.gz", "archive.tar"},
		{"noext", "noext"},
		{"trailing.", "trailing"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := removeFileExtension(tt.in)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVideoString(t *testing.T) {
	v := Video{Slug: "2018_0514_224801_013"}
	if v.String() != "2018_0514_224801_013" {
		t.Fatalf("String() = %q", v.String())
	}
}

func TestVideoFile(t *testing.T) {
	v := Video{Slug: "2018_0514_224801_013"}
	if v.File() != "2018_0514_224801_013.MP4" {
		t.Fatalf("File() = %q", v.File())
	}
}

func TestVideoDashStr(t *testing.T) {
	tests := []struct {
		name string
		slug string
		want string
	}{
		{"normal slug", "2018_0514_224801_013", "2018_0514_224801_013"},
		{"longer slug truncates to 20", "2018_0514_224801_013_a_opt", "2018_0514_224801_013"},
		{"short slug returns empty (prod-crash guard)", "shortie", ""},
		{"empty slug returns empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Video{Slug: tt.slug}
			if got := v.DashStr(); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVideoLocation(t *testing.T) {
	t.Run("not flagged returns coords with nil error", func(t *testing.T) {
		v := Video{Lat: 40.0, Lng: -111.0, Flagged: false}
		lat, lng, err := v.Location()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if lat != 40.0 || lng != -111.0 {
			t.Fatalf("got (%v,%v)", lat, lng)
		}
	})
	t.Run("flagged returns coords with error", func(t *testing.T) {
		v := Video{Lat: 40.0, Lng: -111.0, Flagged: true}
		_, _, err := v.Location()
		if err == nil {
			t.Fatal("expected error for flagged video")
		}
	})
}

func TestVideoToDate(t *testing.T) {
	cases := []struct {
		slug string
		want time.Time
	}{
		{"2018_0514_224801_013", time.Date(2018, 5, 14, 22, 48, 1, 0, time.UTC)},
		{"2018_0514_224801_013_a_opt", time.Date(2018, 5, 14, 22, 48, 1, 0, time.UTC)},
		// A season-2 piece is dated at its offset into the original.
		{"20260605144519_000001_s0000", time.Date(2026, 6, 5, 14, 45, 19, 0, time.UTC)},
		{"20260605144519_000001_s0180", time.Date(2026, 6, 5, 14, 48, 19, 0, time.UTC)},
		// Neither shape: no date, rather than a season-1 slice of the digits.
		{"20260605144519_000001", time.Time{}},
		{"", time.Time{}},
	}
	for _, tc := range cases {
		if got := (Video{Slug: tc.slug}).toDate(); !got.Equal(tc.want) {
			t.Errorf("toDate(%q) = %v, want %v", tc.slug, got, tc.want)
		}
	}
}

// A second of s2fast clip is six seconds of road; real-time clips, and a
// Video that never came from the DB (Speed 0), read as themselves.
func TestRealElapsed(t *testing.T) {
	for speed, want := range map[int]time.Duration{0: 10 * time.Second, 1: 10 * time.Second, 6: time.Minute} {
		if got := (Video{Speed: speed}).realElapsed(10 * time.Second); got != want {
			t.Errorf("Speed %d: realElapsed(10s) = %v, want %v", speed, got, want)
		}
	}
}

func TestFilmedAt(t *testing.T) {
	start := time.Date(2026, 6, 5, 14, 45, 19, 0, time.UTC)
	for _, tc := range []struct {
		v    Video
		want time.Time
	}{
		{Video{DateFilmed: start, Speed: 1}, start.Add(30 * time.Second)},
		{Video{DateFilmed: start, Speed: 6}, start.Add(3 * time.Minute)},
		{Video{}, time.Time{}},
	} {
		if got := tc.v.FilmedAt(30 * time.Second); !got.Equal(tc.want) {
			t.Errorf("Speed %d: FilmedAt(30s) = %v, want %v", tc.v.Speed, got, tc.want)
		}
	}
}
