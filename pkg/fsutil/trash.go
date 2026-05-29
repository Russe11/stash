package fsutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// MoveToTrash moves a file or directory to a custom trash directory.
// The destination is guaranteed not to already exist: if a file with the same
// name is already in the trash, a timestamp is appended, and if that still
// collides (e.g. several same-named files deleted within the same second) an
// incrementing counter is appended until a free name is found.
// Returns the destination path where the file was moved to.
//
// The trash is a recovery store, so it must never silently lose data. Note that
// SafeMove (used below) intentionally overwrites its destination for other
// callers (regenerating sprites/previews, moving DB backups), so trash safety
// is enforced here by selecting a non-existent destination rather than by
// changing SafeMove's contract.
func MoveToTrash(sourcePath string, trashPath string) (string, error) {
	// Get absolute path for the source
	absSourcePath, err := filepath.Abs(sourcePath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Ensure trash directory exists
	if err := os.MkdirAll(trashPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create trash directory: %w", err)
	}

	// Choose a destination that does not already exist in the trash.
	destPath := uniqueTrashDest(trashPath, filepath.Base(absSourcePath))

	// Move the file to trash using SafeMove to support cross-filesystem moves
	if err := SafeMove(absSourcePath, destPath); err != nil {
		return "", fmt.Errorf("failed to move to trash: %w", err)
	}

	return destPath, nil
}

// uniqueTrashDest returns a path inside trashPath for baseName that does not
// already exist on disk. It tries, in order: the unchanged baseName, then
// "<name>_<timestamp><ext>", then "<name>_<timestamp>_<n><ext>" with an
// incrementing n, returning the first candidate that is free.
//
// This relies on an os.Stat check rather than an atomic reservation, so a
// genuinely concurrent move of an identically-named file is theoretically still
// racy; closing that fully would require a non-portable RENAME_NOREPLACE. In
// practice deletions are committed serially, and this eliminates the previous
// data-loss bug where a 1-second-resolution timestamp was the only collision
// guard (same-second same-name deletes overwrote each other).
func uniqueTrashDest(trashPath, baseName string) string {
	destPath := filepath.Join(trashPath, baseName)
	if pathIsFree(destPath) {
		return destPath
	}

	ext := filepath.Ext(baseName)
	nameWithoutExt := baseName[:len(baseName)-len(ext)]
	timestamp := time.Now().Format("20060102-150405")

	for i := 0; ; i++ {
		var candidate string
		if i == 0 {
			candidate = fmt.Sprintf("%s_%s%s", nameWithoutExt, timestamp, ext)
		} else {
			candidate = fmt.Sprintf("%s_%s_%d%s", nameWithoutExt, timestamp, i, ext)
		}

		destPath = filepath.Join(trashPath, candidate)
		if pathIsFree(destPath) {
			return destPath
		}
	}
}

// pathIsFree reports whether path definitively does not exist. A stat error
// other than "not exist" (e.g. a permission error) is treated as not-free, so
// the caller conservatively moves on to the next candidate rather than risk
// overwriting something it cannot see.
func pathIsFree(path string) bool {
	_, err := os.Stat(path)
	return errors.Is(err, os.ErrNotExist)
}
