package api

import (
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
)

// TestValidateFolderPath pins the move/relocate security boundary: a destination must resolve to a
// configured stash library path. An out-of-library destination is the dangerous case (it could move
// files anywhere the server process can write), so it must be rejected.
func TestValidateFolderPath(t *testing.T) {
	mr := &mutationResolver{&Resolver{}}
	paths := config.StashConfigs{
		{Path: "/library"},
		{Path: "/media/extra"},
	}

	cases := []struct {
		name      string
		path      string
		wantError bool
	}{
		{"within first library", "/library/movies", false},
		{"library root itself", "/library", false},
		{"within second library", "/media/extra/clips/2026", false},
		{"outside any library", "/etc", true},
		{"sibling of a library", "/library-other", true},
		{"empty path", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := mr.validateFolderPath(paths, c.path)
			if c.wantError && err == nil {
				t.Errorf("validateFolderPath(%q) = nil, want an error", c.path)
			}
			if !c.wantError && err != nil {
				t.Errorf("validateFolderPath(%q) = %v, want nil", c.path, err)
			}
		})
	}
}

// TestDestinationInSourceSubtree covers the move-cycle guard. destAncestors is
// GetManyParentFolderIDs([destParentID]) — the destination's ancestors excluding itself — so the
// source is an ancestor of the destination (a cycle) iff its id appears there.
func TestDestinationInSourceSubtree(t *testing.T) {
	src := models.FolderID(1)

	cases := []struct {
		name          string
		destAncestors [][]models.FolderID
		want          bool
	}{
		{"source is an ancestor of dest", [][]models.FolderID{{3, 2, 1}}, true},
		{"source not among dest ancestors", [][]models.FolderID{{3, 2}}, false},
		{"dest has no ancestors (top-level)", [][]models.FolderID{{}}, false},
		{"no ancestor rows returned", [][]models.FolderID{}, false},
		{"nil ancestors", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := destinationInSourceSubtree(src, c.destAncestors); got != c.want {
				t.Errorf("destinationInSourceSubtree(%d, %v) = %v, want %v", src, c.destAncestors, got, c.want)
			}
		})
	}
}
