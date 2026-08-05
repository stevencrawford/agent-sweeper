package testutil

import (
	"path/filepath"
	"testing"
)

func TestUnderRootAcceptsDescendant(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "opencode.db")
	if !UnderRoot(root, inside) {
		t.Fatalf("UnderRoot(%q, %q) = false, want true", root, inside)
	}
}

func TestUnderRootRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	sibling := filepath.Join(filepath.Dir(root), "other")
	real := filepath.Join(filepath.Dir(filepath.Dir(root)), "home", ".copilot")
	if UnderRoot(root, sibling) {
		t.Fatalf("UnderRoot(%q, %q) = true, want false (sibling dir)", root, sibling)
	}
	if UnderRoot(root, real) {
		t.Fatalf("UnderRoot(%q, %q) = true, want false (a real ~/.copilot path)", root, real)
	}
}

// TestUnderRootAcceptsBoundary documents that a path equal to the root is
// inside: the guard refuses paths that escape the fixture root, and the root
// itself never contains another agent's data.
func TestUnderRootAcceptsBoundary(t *testing.T) {
	root := t.TempDir()
	if !UnderRoot(root, root) {
		t.Fatal("UnderRoot(root, root) must be true: the root is the fixture boundary")
	}
}

func TestWriteAndSeedStayUnderRoot(t *testing.T) {
	root := t.TempDir()
	path := WriteFile(t, root, "x.jsonl", []byte("x"))
	if !UnderRoot(root, path) {
		t.Fatalf("written path %q escapes root %q", path, root)
	}
}