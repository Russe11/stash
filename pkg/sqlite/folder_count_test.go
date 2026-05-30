//go:build integration
// +build integration

package sqlite_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stashapp/stash/pkg/models"
)

func depthLabel(d *int) string {
	if d == nil {
		return "nil"
	}
	return fmt.Sprintf("%d", *d)
}

// TestFolderCountInTreesMatchSingle is the correctness oracle for the batched dataloader path: for
// every fixture folder (plus a non-existent id, which must yield 0) and a range of depths, the
// batched CountScenesInTrees / CountImagesInTrees / TotalSizeInTrees must return exactly what the
// per-folder CountScenesInTree / CountImagesInTree / TotalSizeInTree return. This pins the windowed
// recursive CTE's per-root GROUP BY against the proven single-folder query.
func TestFolderCountInTreesMatchSingle(t *testing.T) {
	runWithRollbackTxn(t, "batched folder counts match single", func(t *testing.T, ctx context.Context) {
		qb := db.Folder

		ids := make([]models.FolderID, 0, totalFolders+1)
		for i := 0; i < totalFolders; i++ {
			ids = append(ids, folderIDs[i])
		}
		bogus := models.FolderID(999999) // not a real folder; every count must be 0
		ids = append(ids, bogus)

		depths := []*int{nil}
		for d := 0; d <= 3; d++ {
			dd := d
			depths = append(depths, &dd)
		}

		for _, depth := range depths {
			scenes, err := qb.CountScenesInTrees(ctx, ids, depth)
			if err != nil {
				t.Fatalf("CountScenesInTrees(depth=%s): %v", depthLabel(depth), err)
			}
			images, err := qb.CountImagesInTrees(ctx, ids, depth)
			if err != nil {
				t.Fatalf("CountImagesInTrees(depth=%s): %v", depthLabel(depth), err)
			}
			// Every requested id must have an entry (0 for empty/non-existent subtrees).
			if len(scenes) != len(ids) || len(images) != len(ids) {
				t.Fatalf("depth=%s: batched maps must have an entry per id (scenes=%d images=%d, want %d)",
					depthLabel(depth), len(scenes), len(images), len(ids))
			}

			for _, id := range ids {
				wantScenes, err := qb.CountScenesInTree(ctx, id, depth)
				if err != nil {
					t.Fatalf("CountScenesInTree(%d, %s): %v", id, depthLabel(depth), err)
				}
				if scenes[id] != wantScenes {
					t.Errorf("scenes depth=%s id=%d: batched %d != single %d", depthLabel(depth), id, scenes[id], wantScenes)
				}

				wantImages, err := qb.CountImagesInTree(ctx, id, depth)
				if err != nil {
					t.Fatalf("CountImagesInTree(%d, %s): %v", id, depthLabel(depth), err)
				}
				if images[id] != wantImages {
					t.Errorf("images depth=%s id=%d: batched %d != single %d", depthLabel(depth), id, images[id], wantImages)
				}
			}
		}

		sizes, err := qb.TotalSizeInTrees(ctx, ids)
		if err != nil {
			t.Fatalf("TotalSizeInTrees: %v", err)
		}
		if len(sizes) != len(ids) {
			t.Fatalf("batched size map must have an entry per id (got %d, want %d)", len(sizes), len(ids))
		}
		for _, id := range ids {
			want, err := qb.TotalSizeInTree(ctx, id)
			if err != nil {
				t.Fatalf("TotalSizeInTree(%d): %v", id, err)
			}
			if sizes[id] != want {
				t.Errorf("size id=%d: batched %d != single %d", id, sizes[id], want)
			}
		}

		// The non-existent id is the empty-subtree sentinel: 0 everywhere.
		if sizes[bogus] != 0 {
			t.Errorf("non-existent folder size = %d, want 0", sizes[bogus])
		}
	})
}

// TestFolderCountScenesInTree verifies the recursive scene count and its depth semantics against
// the fixture hierarchy: scene files live in folderIdxWithSceneFiles, a child of
// folderIdxForObjectFiles. So a depth-0 count on the parent sees none, while an unlimited count
// descends to them — proving the recursive CTE and the depth limit both work.
func TestFolderCountScenesInTree(t *testing.T) {
	runWithRollbackTxn(t, "folder scene counts", func(t *testing.T, ctx context.Context) {
		qb := db.Folder
		parent := folderIDs[folderIdxForObjectFiles] // scene files are NOT directly here
		leaf := folderIDs[folderIdxWithSceneFiles]   // ...they are in this child

		zero := func(d int) *int { return &d }

		// depth 0 on the parent: only scenes directly in the parent (none).
		directParent, err := qb.CountScenesInTree(ctx, parent, zero(0))
		if err != nil {
			t.Fatalf("CountScenesInTree(parent, 0): %v", err)
		}

		// unlimited (nil) on the parent: descends into the scene-files child.
		recursive, err := qb.CountScenesInTree(ctx, parent, nil)
		if err != nil {
			t.Fatalf("CountScenesInTree(parent, nil): %v", err)
		}

		// depth 0 on the leaf: its direct scenes.
		directLeaf, err := qb.CountScenesInTree(ctx, leaf, zero(0))
		if err != nil {
			t.Fatalf("CountScenesInTree(leaf, 0): %v", err)
		}

		// depth 1 on the parent: parent + one level of descendants reaches the leaf's scenes.
		depth1, err := qb.CountScenesInTree(ctx, parent, zero(1))
		if err != nil {
			t.Fatalf("CountScenesInTree(parent, 1): %v", err)
		}

		if directLeaf <= 0 {
			t.Fatalf("expected the scene-files folder to contain scenes, got %d", directLeaf)
		}
		// the key correctness proof: recursion descends, and depth 0 excludes the child.
		if recursive <= directParent {
			t.Errorf("recursive count (%d) should exceed the depth-0 parent count (%d) — recursion didn't descend", recursive, directParent)
		}
		// depth 1 reaches the immediate child where the scenes are, matching the unlimited count.
		if depth1 != recursive {
			t.Errorf("depth-1 count (%d) should equal the unlimited count (%d) for scenes one level down", depth1, recursive)
		}

		// total_size recurses: the parent (which includes the leaf's files) must be >= the leaf's,
		// and never negative. (We don't assert a positive value: the fixture's file sizes are
		// fileIndex*10, and the object files in this subtree happen to sum to 0.)
		leafSize, err := qb.TotalSizeInTree(ctx, leaf)
		if err != nil {
			t.Fatalf("TotalSizeInTree(leaf): %v", err)
		}
		parentSize, err := qb.TotalSizeInTree(ctx, parent)
		if err != nil {
			t.Fatalf("TotalSizeInTree(parent): %v", err)
		}
		if leafSize < 0 || parentSize < 0 {
			t.Errorf("total size should never be negative: leaf=%d parent=%d", leafSize, parentSize)
		}
		if parentSize < leafSize {
			t.Errorf("recursive parent size (%d) should be >= leaf size (%d)", parentSize, leafSize)
		}

		// image_count recurses the same way (the fixture has an images folder elsewhere; just assert
		// it runs and is non-negative and respects depth-0 <= unlimited).
		imgRecursive, err := qb.CountImagesInTree(ctx, parent, nil)
		if err != nil {
			t.Fatalf("CountImagesInTree(parent, nil): %v", err)
		}
		imgDirect, err := qb.CountImagesInTree(ctx, parent, zero(0))
		if err != nil {
			t.Fatalf("CountImagesInTree(parent, 0): %v", err)
		}
		if imgRecursive < imgDirect {
			t.Errorf("recursive image count (%d) should be >= depth-0 (%d)", imgRecursive, imgDirect)
		}
	})
}
