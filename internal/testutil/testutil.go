// Package testutil supplies the throwaway fixture seam for tests that exercise
// the destructive sweep path. Every session-store path a test builds flows from
// a single fixture root returned by StoreRoot, so a real agent store can never
// be the target of a deletion: a path that escapes the root is refused by
// UnderRoot, and a test must assert that before it deletes anything.
package testutil

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// StoreRoot returns the throwaway directory this test's session-store fixtures
// live under. It is the single seam through which fixture roots enter engine and
// protection code: build every store path from StoreRoot, never from a real
// agent home. The never-use-real-store rule (ticket 12) makes this the only root
// a test may use.
func StoreRoot(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// UnderRoot reports whether path is confined to the fixture root. The deletion
// tests assert it is true before touching anything, so a mis-typed fixture
// path (e.g. a real "~/.copilot/...") fails loudly instead of sweeping a real
// session store.
func UnderRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// assertUnderRoot fails the test unless UnderRoot(root, path) is true.
func assertUnderRoot(t *testing.T, root, path string) {
	t.Helper()
	if !UnderRoot(root, path) {
		t.Fatalf("fixture path %q escapes fixture root %q: a sweep test may only target its own temp stores", path, root)
	}
}

// Store asserts name lives under root and returns its absolute path. No file is
// created; it is the one place fixture-relative paths are resolved, so engine
// and protection code only ever see paths confined to the root.
func Store(root, name string) string {
	return filepath.Join(root, name)
}

// SeedDB creates a schema-minimal SQLite file at root/name and returns its
// path. The schema is deliberately minimal — just the tables a plan's DELETE
// needs — not a faithful mirror of any agent store (ticket 12, scheme-minimal).
func SeedDB(t *testing.T, root, name, schema string) string {
	t.Helper()
	path := Store(root, name)
	assertUnderRoot(t, root, path)
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("seed schema: %v", err)
	}
	return path
}

// WriteFile writes data to root/name and returns the path, failing the test on
// error or on any path that escapes the root.
func WriteFile(t *testing.T, root, name string, data []byte) string {
	t.Helper()
	path := Store(root, name)
	assertUnderRoot(t, root, path)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// ReadFile returns the contents of path, failing the test on error.
func ReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// CountRows returns how many rows path yields under the query.
func CountRows(t *testing.T, path, query string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}
