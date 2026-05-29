package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMoveToTrash_SameBasenameCollision is the regression guard for the
// trash-overwrite data-loss bug: several files sharing a basename, deleted
// within the same second, must all survive in the trash with their original
// contents. Before the fix, the 1-second-resolution timestamp was the only
// collision guard, so the third same-named file overwrote the second.
func TestMoveToTrash_SameBasenameCollision(t *testing.T) {
	root := t.TempDir()
	trash := filepath.Join(root, "trash")

	// Three distinct files, all named cover.jpg, in different source folders.
	contents := []string{"alpha", "bravo", "charlie"}
	var dests []string
	for i, content := range contents {
		srcDir := filepath.Join(root, "src", filepath.Base(t.Name()), string(rune('a'+i)))
		if err := os.MkdirAll(srcDir, 0o755); err != nil {
			t.Fatalf("mkdir src: %v", err)
		}
		src := filepath.Join(srcDir, "cover.jpg")
		if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
			t.Fatalf("write src: %v", err)
		}

		dest, err := MoveToTrash(src, trash)
		if err != nil {
			t.Fatalf("MoveToTrash(%q): %v", src, err)
		}
		dests = append(dests, dest)
	}

	// All destination paths must be distinct.
	seen := map[string]bool{}
	for _, d := range dests {
		if seen[d] {
			t.Fatalf("duplicate trash destination %q — a file was overwritten", d)
		}
		seen[d] = true
	}

	// Every original content must be recoverable from the trash exactly once.
	got := map[string]bool{}
	for _, d := range dests {
		b, err := os.ReadFile(d)
		if err != nil {
			t.Fatalf("read trash dest %q: %v", d, err)
		}
		got[string(b)] = true
	}
	for _, want := range contents {
		if !got[want] {
			t.Fatalf("content %q was lost from the trash (overwritten); recovered set = %v", want, got)
		}
	}
}

// TestMoveToTrash_HappyPath confirms a plain move into an empty trash keeps the
// original basename and contents, and removes the source.
func TestMoveToTrash_HappyPath(t *testing.T) {
	root := t.TempDir()
	trash := filepath.Join(root, "trash")

	src := filepath.Join(root, "scene.mp4")
	if err := os.WriteFile(src, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	dest, err := MoveToTrash(src, trash)
	if err != nil {
		t.Fatalf("MoveToTrash: %v", err)
	}

	if base := filepath.Base(dest); base != "scene.mp4" {
		t.Errorf("expected basename scene.mp4, got %q", base)
	}
	if b, err := os.ReadFile(dest); err != nil || string(b) != "payload" {
		t.Errorf("trash dest content = %q, err = %v; want %q", string(b), err, "payload")
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source should have been moved away, stat err = %v", err)
	}
}
