package api

import (
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
)

func TestViewerPreferencesResolverRoundTrip(t *testing.T) {
	c := config.InitializeEmpty()
	c.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))

	r := &Resolver{}

	initial, err := r.Query().ViewerPreferences(testCtx)
	if err != nil {
		t.Fatalf("ViewerPreferences initial: %v", err)
	}
	if got := initial.StashTv.Home.ExcludedTagIDs; len(got) != 0 {
		t.Fatalf("initial excludedTagIDs = %v, want empty", got)
	}

	updated, err := r.Mutation().UpdateViewerPreferences(testCtx, ViewerPreferencesInput{
		StashTv: &StashTVViewerPreferencesInput{
			Home: &StashTVHomePreferencesInput{
				ExcludedTagIDs: []string{"5", "4", "5", "", " 3 "},
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateViewerPreferences: %v", err)
	}

	want := []string{"5", "4", "3"}
	if got := updated.StashTv.Home.ExcludedTagIDs; !equalStringSlices(got, want) {
		t.Fatalf("updated excludedTagIDs = %v, want %v", got, want)
	}

	reloaded, err := r.Query().ViewerPreferences(testCtx)
	if err != nil {
		t.Fatalf("ViewerPreferences reloaded: %v", err)
	}
	if got := reloaded.StashTv.Home.ExcludedTagIDs; !equalStringSlices(got, want) {
		t.Fatalf("reloaded excludedTagIDs = %v, want %v", got, want)
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
