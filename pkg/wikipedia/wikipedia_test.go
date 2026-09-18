package wikipedia

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serveSummary points summaryURL at an httptest server running h for the
// duration of the test. Rebinding a package var, so not safe for t.Parallel.
func serveSummary(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	orig := summaryURL
	summaryURL = srv.URL + "/"
	t.Cleanup(func() { summaryURL = orig })
}

// serveJSON is serveSummary for the common case: one fixed status and body.
// The request path is captured so a test can assert what title was asked for.
func serveJSON(t *testing.T, status int, body string) *string {
	t.Helper()
	var path string
	serveSummary(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.EscapedPath()
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	})
	return &path
}

// The summary paragraph runs for several sentences of history and
// demographics; only the first one says what the place is, and only the first
// one fits a chat message.
func TestSummary_ReturnsTheFirstSentence(t *testing.T) {
	serveJSON(t, http.StatusOK, `{"type":"standard","extract":"Dillon is a home rule municipality in Summit County, Colorado. The population was 1,064 at the 2020 census. It sits on a reservoir."}`)

	got, err := REST{}.Summary(context.Background(), "Dillon, Colorado")
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if want := "Dillon is a home rule municipality in Summit County, Colorado."; got != want {
		t.Errorf("Summary = %q, want %q", got, want)
	}
}

// Titles carry spaces as underscores and have to survive as one path segment —
// a town with an apostrophe in its name must still resolve, and a space left
// raw would be a 400 from the edge cache rather than an article. The comma
// every US place title carries is escaped rather than left literal; both forms
// answer 200 (checked against the live endpoint), so this pins the shape
// rather than a requirement.
func TestSummary_EscapesTheTitleIntoOnePathSegment(t *testing.T) {
	cases := []struct{ title, want string }{
		{"Dillon, Colorado", "/Dillon%2C_Colorado"},
		{"Coeur d'Alene, Idaho", "/Coeur_d%27Alene%2C_Idaho"},
		{"Truth or Consequences, New Mexico", "/Truth_or_Consequences%2C_New_Mexico"},
	}
	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			path := serveJSON(t, http.StatusOK, `{"type":"standard","extract":"A place."}`)
			if _, err := (REST{}).Summary(context.Background(), tc.title); err != nil {
				t.Fatalf("Summary: %v", err)
			}
			if *path != tc.want {
				t.Errorf("requested %q, want %q", *path, tc.want)
			}
		})
	}
}

// The two "nothing to say" shapes both have to arrive as ErrNoArticle, because
// the caller says something different for them than for a broken lookup. A
// disambiguation page is the subtle one: it is a 200 whose extract reads like
// prose, so type is the only thing that distinguishes it.
func TestSummary_NoArticle(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"missing page", http.StatusNotFound, `{"type":"https://mediawiki.org/wiki/HyperSwitch/errors/not_found"}`},
		{"disambiguation", http.StatusOK, `{"type":"disambiguation","extract":"Dillon may refer to:"}`},
		{"empty extract", http.StatusOK, `{"type":"standard","extract":"   "}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			serveJSON(t, tc.status, tc.body)
			_, err := REST{}.Summary(context.Background(), "Dillon, Colorado")
			if !errors.Is(err, ErrNoArticle) {
				t.Errorf("err = %v, want ErrNoArticle", err)
			}
		})
	}
}

func TestSummary_SurfacesFailures(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{"server error", http.StatusInternalServerError, `{}`, "status 500"},
		{"rate limited", http.StatusTooManyRequests, `{}`, "status 429"},
		{"malformed json", http.StatusOK, `{"extract":`, "unexpected EOF"},
		{"not json at all", http.StatusOK, `<html>down for maintenance</html>`, "invalid character"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			serveJSON(t, tc.status, tc.body)
			_, err := REST{}.Summary(context.Background(), "Dillon, Colorado")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want one containing %q", err, tc.wantErr)
			}
			// A transport or decode failure is not "no article" — the caller
			// apologizes for one and stays quiet about the other.
			if errors.Is(err, ErrNoArticle) {
				t.Errorf("err = %v, want it distinguishable from ErrNoArticle", err)
			}
		})
	}
}

// Wikimedia's policy is that an unidentified client may be refused or throttled,
// and Go's default UA is exactly that — so the header is load-bearing, and its
// absence would show up as intermittent 403s in production and nowhere else.
func TestSummary_IdentifiesItself(t *testing.T) {
	var ua string
	serveSummary(t, func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		_, _ = io.WriteString(w, `{"type":"standard","extract":"A place."}`)
	})
	if _, err := (REST{}).Summary(context.Background(), "Dillon, Colorado"); err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if !strings.Contains(ua, "tripbot") {
		t.Errorf("User-Agent = %q, want one naming the bot", ua)
	}
}

// Place summaries are full of periods that do not end a sentence. Cutting at
// the first one turns "Ely is a city on U.S. Route 50" into "Ely is a city on
// U.S." — still grammatical-looking, which is why nothing downstream would
// catch it.
func TestFirstSentence(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			"plain",
			"Dillon is a town in Colorado. It has a reservoir.",
			"Dillon is a town in Colorado.",
		},
		{
			"abbreviation mid-sentence",
			"Ely is a city on U.S. Route 50, the Loneliest Road. It was founded as a stagecoach stop.",
			"Ely is a city on U.S. Route 50, the Loneliest Road.",
		},
		{
			"saint abbreviation",
			"St. Louis is a city in Missouri. It sits on the Mississippi.",
			"St. Louis is a city in Missouri.",
		},
		{
			"an initial",
			"The town is named for J. R. Smith, a rancher. He arrived in 1881.",
			"The town is named for J. R. Smith, a rancher.",
		},
		{
			"a decimal",
			"The city covers 3.5 square miles. Most of it is desert.",
			"The city covers 3.5 square miles.",
		},
		{
			"no sentence break at all",
			"A hamlet in Nevada",
			"A hamlet in Nevada",
		},
		{
			"one sentence, ending the text",
			"A hamlet in Nevada.",
			"A hamlet in Nevada.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstSentence(tc.in); got != tc.want {
				t.Errorf("firstSentence = %q, want %q", got, tc.want)
			}
		})
	}
}
