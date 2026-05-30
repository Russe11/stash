//go:build integration
// +build integration

package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sqlite"
)

// makeFileOnDisk writes a real file at <parent.Path>/<basename> with the given contents and inserts a
// matching basic `files` row whose parent_folder_id points at parent. It returns the created file,
// whose ID has been assigned by the store. This mirrors makeFolderOnDisk (in
// resolver_query_deleted_integration_test.go) but for the file level, so MoveFiles has real rows + a
// real on-disk file to move.
func makeFileOnDisk(t *testing.T, db *sqlite.Database, parent *models.Folder, basename, contents string) *models.BaseFile {
	t.Helper()

	onDisk := filepath.Join(parent.Path, basename)
	if err := os.WriteFile(onDisk, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing file %s: %v", onDisk, err)
	}

	f := &models.BaseFile{
		Basename:       basename,
		ParentFolderID: parent.ID,
		Size:           int64(len(contents)),
		DirEntry:       models.DirEntry{ModTime: time.Unix(0, 0).UTC()},
	}
	repo := db.Repository()
	if err := repo.WithTxn(context.Background(), func(ctx context.Context) error {
		return repo.File.Create(ctx, f)
	}); err != nil {
		t.Fatalf("creating file row %s: %v", onDisk, err)
	}
	return f
}

// reloadFile re-reads a file row by id in a fresh read txn, so assertions see the committed state
// rather than the in-memory struct the mover mutated. The resolved Path is derived from the parent
// folder + basename by the store, so it reflects the post-move location.
func reloadFile(t *testing.T, db *sqlite.Database, id models.FileID) *models.BaseFile {
	t.Helper()
	repo := db.Repository()
	var f *models.BaseFile
	if err := repo.WithReadTxn(context.Background(), func(ctx context.Context) error {
		files, err := repo.File.Find(ctx, id)
		if err != nil {
			return err
		}
		f = files[0].Base()
		return nil
	}); err != nil {
		t.Fatalf("reloading file %d: %v", id, err)
	}
	return f
}

// TestMoveFilesResolverByIDEndToEnd drives the MoveFiles mutation resolver against a real sqlite DB
// and a real filesystem through the stashPaths seam, asserting the by-id (DestinationFolderID) branch:
// the file row is re-parented to the destination folder and its on-disk file is actually renamed
// there, while a sibling file left behind in the source is untouched. This is the file-move
// filesystem path that the helper-level unit tests (validateFolderPath / destinationInSourceSubtree)
// cannot reach.
func TestMoveFilesResolverByIDEndToEnd(t *testing.T) {
	resolver, db, libraryRoot := newMoveFolderTestResolver(t)
	m := resolver.Mutation().(*mutationResolver)
	ctx := context.Background()

	// library/ (root) with two children: src/ (holds the files) and dst/ (move target).
	lib := makeFolderOnDisk(t, db, libraryRoot, nil)
	src := makeFolderOnDisk(t, db, filepath.Join(libraryRoot, "src"), &lib.ID)
	dst := makeFolderOnDisk(t, db, filepath.Join(libraryRoot, "dst"), &lib.ID)

	// Two files under src/: one we move, one that must stay put.
	moving := makeFileOnDisk(t, db, src, "clip.mp4", "primary contents")
	staying := makeFileOnDisk(t, db, src, "other.mp4", "sibling contents")

	ok, err := m.MoveFiles(ctx, MoveFilesInput{
		Ids:                 []string{moving.ID.String()},
		DestinationFolderID: strptr(dst.ID.String()),
	})
	if err != nil {
		t.Fatalf("MoveFiles by id: %v", err)
	}
	if !ok {
		t.Fatalf("MoveFiles returned ok=false without error")
	}

	wantPath := filepath.Join(libraryRoot, "dst", "clip.mp4")
	oldPath := filepath.Join(libraryRoot, "src", "clip.mp4")

	// DB: the file row now lives under dst with parent = dst, basename unchanged.
	moved := reloadFile(t, db, moving.ID)
	if moved.ParentFolderID != dst.ID {
		t.Errorf("moved file parent = %d, want %d", moved.ParentFolderID, dst.ID)
	}
	if moved.Basename != "clip.mp4" {
		t.Errorf("moved file basename = %q, want %q", moved.Basename, "clip.mp4")
	}
	if moved.Path != wantPath {
		t.Errorf("moved file path = %q, want %q", moved.Path, wantPath)
	}

	// Filesystem: the file really moved.
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("expected file at new path %s: %v", wantPath, err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("expected old path %s to be gone, stat err = %v", oldPath, err)
	}

	// The sibling file was not touched, in the DB or on disk.
	sib := reloadFile(t, db, staying.ID)
	if sib.ParentFolderID != src.ID {
		t.Errorf("sibling file parent = %d, want %d (untouched)", sib.ParentFolderID, src.ID)
	}
	if _, err := os.Stat(filepath.Join(libraryRoot, "src", "other.mp4")); err != nil {
		t.Errorf("sibling file should remain on disk: %v", err)
	}
}

// NOTE: the DestinationBasename (rename-on-move) branch is intentionally NOT covered here. When a
// basename is supplied the resolver calls validateFileExtension, which still reads
// manager.GetInstance().Config (it is not behind the stashPaths seam), so it panics with "manager not
// initialized" in this no-bootstrap harness. Covering it would require either a manager bootstrap or a
// new extension-list seam (a production change), so per the test's seam-only constraint it is left to a
// follow-up. The no-rename file-move path is fully covered by the by-id and by-path tests here.

// TestMoveFilesResolverByPathEndToEnd drives the by-path (DestinationFolder) branch: instead of an
// existing destination folder id, a destination *path* under the library is given, and the resolver
// must validate it is within the library, get-or-create the destination folder row + directory, and
// move the file there. This exercises validateFolderPath + GetOrCreateFolderHierarchy +
// CreateFolderHierarchy + the file move together against the real DB and filesystem.
func TestMoveFilesResolverByPathEndToEnd(t *testing.T) {
	resolver, db, libraryRoot := newMoveFolderTestResolver(t)
	m := resolver.Mutation().(*mutationResolver)
	ctx := context.Background()

	// Seed the library root folder row (the get-or-create walk needs it as the parent of the new
	// destination subfolder) and a src/ folder holding the file.
	lib := makeFolderOnDisk(t, db, libraryRoot, nil)
	src := makeFolderOnDisk(t, db, filepath.Join(libraryRoot, "src"), &lib.ID)
	moving := makeFileOnDisk(t, db, src, "clip.mp4", "by path contents")

	// Destination is a NOT-yet-existing subfolder of the library, given by path. The resolver must
	// create both the folder row and the directory.
	destPath := filepath.Join(libraryRoot, "created")
	ok, err := m.MoveFiles(ctx, MoveFilesInput{
		Ids:               []string{moving.ID.String()},
		DestinationFolder: strptr(destPath),
	})
	if err != nil {
		t.Fatalf("MoveFiles by path: %v", err)
	}
	if !ok {
		t.Fatalf("MoveFiles returned ok=false without error")
	}

	wantPath := filepath.Join(destPath, "clip.mp4")
	oldPath := filepath.Join(libraryRoot, "src", "clip.mp4")

	// The destination directory was created on disk.
	if info, statErr := os.Stat(destPath); statErr != nil || !info.IsDir() {
		t.Errorf("expected destination dir %s to exist: err=%v", destPath, statErr)
	}

	// The destination folder row was created and the file re-parented to it.
	moved := reloadFile(t, db, moving.ID)
	if moved.Basename != "clip.mp4" {
		t.Errorf("moved file basename = %q, want %q", moved.Basename, "clip.mp4")
	}
	if moved.Path != wantPath {
		t.Errorf("moved file path = %q, want %q", moved.Path, wantPath)
	}
	destFolder := reloadFolder(t, db, moved.ParentFolderID)
	if destFolder == nil || destFolder.Path != destPath {
		t.Errorf("file parent folder = %v, want a folder row at %q", destFolder, destPath)
	}

	// Filesystem: the file really moved.
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("expected file at new path %s: %v", wantPath, err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("expected old path %s to be gone, stat err = %v", oldPath, err)
	}
}

// TestMoveFilesResolverRejectsOutOfLibrary asserts the move security boundary for the file mover: a
// destination *path* outside any configured library is rejected before any filesystem mutation, so the
// file stays where it was in both the DB and on disk. This drives the injected stashPaths seam (the
// by-path branch reads r.stashPaths.GetStashPaths()).
func TestMoveFilesResolverRejectsOutOfLibrary(t *testing.T) {
	resolver, db, libraryRoot := newMoveFolderTestResolver(t)
	m := resolver.Mutation().(*mutationResolver)
	ctx := context.Background()

	lib := makeFolderOnDisk(t, db, libraryRoot, nil)
	src := makeFolderOnDisk(t, db, filepath.Join(libraryRoot, "src"), &lib.ID)
	moving := makeFileOnDisk(t, db, src, "clip.mp4", "do not move me")

	outside := filepath.Join(t.TempDir(), "outside")
	ok, err := m.MoveFiles(ctx, MoveFilesInput{
		Ids:               []string{moving.ID.String()},
		DestinationFolder: strptr(outside),
	})
	if err == nil {
		t.Errorf("expected error for out-of-library destination, got nil")
	}
	if ok {
		t.Errorf("MoveFiles returned ok=true for out-of-library destination")
	}

	// The file is untouched, in the filesystem and the DB.
	srcPath := filepath.Join(libraryRoot, "src", "clip.mp4")
	if _, statErr := os.Stat(srcPath); statErr != nil {
		t.Errorf("source file should be untouched after rejected move: %v", statErr)
	}
	reloaded := reloadFile(t, db, moving.ID)
	if reloaded.ParentFolderID != src.ID {
		t.Errorf("file row should be untouched, parent = %d, want %d", reloaded.ParentFolderID, src.ID)
	}
}
