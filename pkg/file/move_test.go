package file

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
)

// MoveFolder rejects unsafe sources before touching any store or the filesystem. These guard
// rails run before m.Folders is dereferenced, so a bare Mover (no stores) exercises them.
func TestMoverMoveFolderGuards(t *testing.T) {
	ctx := context.Background()
	m := &Mover{}

	zipFileID := models.FileID(7)
	destParent := &models.Folder{ID: models.FolderID(2), Path: "/library/dest"}

	t.Run("rejects a folder inside a zip", func(t *testing.T) {
		parentID := models.FolderID(1)
		folder := &models.Folder{
			ID:             models.FolderID(10),
			Path:           "/library/zip/sub",
			ParentFolderID: &parentID,
			DirEntry:       models.DirEntry{ZipFileID: &zipFileID},
		}
		if err := m.MoveFolder(ctx, folder, destParent, ""); err == nil {
			t.Fatal("expected error moving a folder inside a zip, got nil")
		}
	})

	t.Run("rejects moving a library root folder", func(t *testing.T) {
		// a library root has no parent folder; it must not be moved.
		folder := &models.Folder{ID: models.FolderID(11), Path: "/library", ParentFolderID: nil}
		if err := m.MoveFolder(ctx, folder, destParent, ""); err == nil {
			t.Fatal("expected error moving a library root folder, got nil")
		}
	})
}
