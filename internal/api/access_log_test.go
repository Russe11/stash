package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRedactAPIKeyInURI(t *testing.T) {
	cases := []struct {
		name string
		in   string
		// substrings that must be present / absent in the result
		want    []string
		notWant []string
	}{
		{
			name:    "apikey alone is redacted",
			in:      "/scene/1/stream?apikey=s3cr3t",
			want:    []string{"/scene/1/stream?", "apikey=redacted"},
			notWant: []string{"s3cr3t"},
		},
		{
			name:    "apikey redacted, other params preserved",
			in:      "/scene/1/stream.m3u8?apikey=s3cr3t&resolution=720",
			want:    []string{"apikey=redacted", "resolution=720"},
			notWant: []string{"s3cr3t"},
		},
		{
			name:    "no query is untouched",
			in:      "/graphql",
			want:    []string{"/graphql"},
			notWant: []string{"redacted"},
		},
		{
			name:    "query without apikey is untouched",
			in:      "/scene/1/stream?resolution=720",
			want:    []string{"resolution=720"},
			notWant: []string{"redacted"},
		},
		{
			name:    "repeated apikey is fully redacted",
			in:      "/x?apikey=a&apikey=b",
			notWant: []string{"apikey=a", "apikey=b"},
			want:    []string{"apikey=redacted"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactAPIKeyInURI(tc.in)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("redactAPIKeyInURI(%q) = %q; want substring %q", tc.in, got, w)
				}
			}
			for _, nw := range tc.notWant {
				if strings.Contains(got, nw) {
					t.Errorf("redactAPIKeyInURI(%q) = %q; must not contain %q", tc.in, got, nw)
				}
			}
		})
	}
}

// TestRedactAPIKeyAccessLogLeavesURLIntact verifies the middleware sanitises
// only r.RequestURI — the parsed r.URL (read by auth and HLS/DASH manifest
// handlers) must still carry the real key.
func TestRedactAPIKeyAccessLogLeavesURLIntact(t *testing.T) {
	const key = "s3cr3t"
	r := httptest.NewRequest(http.MethodGet, "/scene/1/stream.m3u8?apikey="+key+"&resolution=720", nil)
	// httptest.NewRequest derives RequestURI from the target; assert our premise.
	if !strings.Contains(r.RequestURI, key) {
		t.Fatalf("precondition: RequestURI %q should contain the key", r.RequestURI)
	}

	var seen *http.Request
	redactAPIKeyAccessLog(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		seen = req
	})).ServeHTTP(httptest.NewRecorder(), r)

	if strings.Contains(seen.RequestURI, key) {
		t.Errorf("RequestURI still leaks the key: %q", seen.RequestURI)
	}
	if !strings.Contains(seen.RequestURI, "apikey=redacted") {
		t.Errorf("RequestURI not redacted: %q", seen.RequestURI)
	}
	if got := seen.URL.Query().Get("apikey"); got != key {
		t.Errorf("r.URL must keep the real key for handlers; got %q want %q", got, key)
	}
}
