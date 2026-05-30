package urlbuilders

import (
	"net/url"
	"strings"
	"testing"
)

const (
	testBaseURL = "http://localhost:9999"
	testSceneID = "42"
	testUpdated = "1700000000"
	testHash    = "deadbeef"
	testAPIKey  = "SECRET"
)

func newTestBuilder() SceneURLBuilder {
	return SceneURLBuilder{
		BaseURL:   testBaseURL,
		SceneID:   testSceneID,
		UpdatedAt: testUpdated,
	}
}

// mediaURLCase describes one media URL builder: the value it produces with and without an apikey, plus
// the bare endpoint path so we can assert the apikey is the only thing that changes.
type mediaURLCase struct {
	name    string
	with    string // URL string built with an apikey configured
	without string // URL string built with no apikey
}

func sceneMediaCases() []mediaURLCase {
	withKey := newTestBuilder()
	noKey := newTestBuilder()
	return []mediaURLCase{
		{"stream", withKey.GetStreamURL(testAPIKey).String(), noKey.GetStreamURL("").String()},
		{"preview", withKey.GetStreamPreviewURL(testAPIKey), noKey.GetStreamPreviewURL("")},
		{"webp", withKey.GetStreamPreviewImageURL(testAPIKey), noKey.GetStreamPreviewImageURL("")},
		{"vtt", withKey.GetSpriteVTTURL(testHash, testAPIKey), noKey.GetSpriteVTTURL(testHash, "")},
		{"sprite", withKey.GetSpriteURL(testHash, testAPIKey), noKey.GetSpriteURL(testHash, "")},
		{"screenshot", withKey.GetScreenshotURL(testAPIKey), noKey.GetScreenshotURL("")},
		{"funscript", withKey.GetFunscriptURL(testAPIKey).String(), noKey.GetFunscriptURL("").String()},
		{"caption", withKey.GetCaptionURL(testAPIKey), noKey.GetCaptionURL("")},
		{"heatmap", withKey.GetInteractiveHeatmapURL(testAPIKey), noKey.GetInteractiveHeatmapURL("")},
	}
}

// TestSceneMediaURLsCarryAPIKey asserts every scene media URL builder appends the apikey query
// parameter when one is configured. The Apple clients authenticate non-header media (screenshot,
// preview, sprite, vtt, caption, heatmap) by the apikey query param, so this must hold uniformly —
// not only for stream/funscript.
func TestSceneMediaURLsCarryAPIKey(t *testing.T) {
	for _, c := range sceneMediaCases() {
		t.Run(c.name, func(t *testing.T) {
			u, err := url.Parse(c.with)
			if err != nil {
				t.Fatalf("parse %q: %v", c.with, err)
			}
			if got := u.Query().Get("apikey"); got != testAPIKey {
				t.Errorf("%s: apikey query param = %q, want %q (url=%s)", c.name, got, testAPIKey, c.with)
			}
		})
	}
}

// TestSceneMediaURLsNoAPIKeyWhenUnset asserts the apikey query param is absent when no key is
// configured, so cookie/no-auth deployments keep clean URLs.
func TestSceneMediaURLsNoAPIKeyWhenUnset(t *testing.T) {
	for _, c := range sceneMediaCases() {
		t.Run(c.name, func(t *testing.T) {
			u, err := url.Parse(c.without)
			if err != nil {
				t.Fatalf("parse %q: %v", c.without, err)
			}
			if _, ok := u.Query()["apikey"]; ok {
				t.Errorf("%s: apikey present but no key configured (url=%s)", c.name, c.without)
			}
		})
	}
}

// TestScreenshotURLPreservesExistingQuery pins that appending the apikey does not clobber the
// pre-existing `?t=` cache-busting param on the screenshot URL: both must survive.
func TestScreenshotURLPreservesExistingQuery(t *testing.T) {
	got := newTestBuilder().GetScreenshotURL(testAPIKey)
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse %q: %v", got, err)
	}
	q := u.Query()
	if q.Get("t") != testUpdated {
		t.Errorf("t param = %q, want %q (url=%s)", q.Get("t"), testUpdated, got)
	}
	if q.Get("apikey") != testAPIKey {
		t.Errorf("apikey param = %q, want %q (url=%s)", q.Get("apikey"), testAPIKey, got)
	}
	// Exactly one '?' so the URL is well-formed when a client later appends more params.
	if strings.Count(got, "?") != 1 {
		t.Errorf("expected exactly one '?' in %s", got)
	}
}

// TestScreenshotURLWithoutKeyUnchanged pins that with no apikey the screenshot URL keeps only its
// original `?t=` param (unchanged behavior).
func TestScreenshotURLWithoutKeyUnchanged(t *testing.T) {
	want := testBaseURL + "/scene/" + testSceneID + "/screenshot?t=" + testUpdated
	if got := newTestBuilder().GetScreenshotURL(""); got != want {
		t.Errorf("GetScreenshotURL(\"\") = %q, want %q", got, want)
	}
}
