package sourcecatalog

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Kuwo's nonstandard quoted strings are decoded as data without executing response code.
func TestKuwoQuotedResponse(t *testing.T) {
	b, err := quotedJSON([]byte(`{'name':'Can\'t go back','artist':'S.H.E','unicode':'\u7231'}`))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]string
	if err = json.Unmarshal(b, &result); err != nil || result["name"] != "Can't go back" || result["unicode"] != "爱" {
		t.Fatalf("%s %v", b, err)
	}
	if _, err := quotedJSON([]byte(`{'x':'unterminated}`)); err == nil {
		t.Fatal("malformed response accepted")
	}
}

// Paginating external search sends the requested page and preserves minute-based durations.
func TestSearchProviderPageAndDuration(t *testing.T) {
	old := client
	client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("page") != "3" {
			t.Fatalf("wrong page: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":1,"data":{"info":[]}}`)), Header: make(http.Header)}, nil
	})}
	defer func() { client = old }()
	if _, err := SearchPage(context.Background(), "kg", "song", 20, 3); err != nil {
		t.Fatal(err)
	}
	if got := parseDuration("03:24"); got != 204*time.Second {
		t.Fatalf("duration: %v", got)
	}
}

// Live provider probes report endpoint availability without requiring a paid plugin.
func TestLiveProviderSearch(t *testing.T) {
	if os.Getenv("LIVE_SEARCH") == "" {
		t.Skip("set LIVE_SEARCH to probe public catalogs")
	}
	for _, p := range []string{"tx", "kg", "kw", "mg"} {
		t.Run(p, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			songs, err := SearchPage(ctx, p, "最近还好吗", 20, 1)
			if err != nil {
				t.Fatal(err)
			}
			if len(songs) == 0 {
				t.Fatal("no results")
			}
			t.Logf("%d songs; first: %s", len(songs), songs[0].Name)
		})
	}
}
