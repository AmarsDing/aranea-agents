package data

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureSQLiteParentDir_fileDSN(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nested", "test.sqlite")
	dsn := "file:" + filepath.ToSlash(dbPath) + "?cache=shared"
	if err := ensureSQLiteParentDir(dsn); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(dir, "nested")); err != nil || !st.IsDir() {
		t.Fatalf("nested dir missing: %v", err)
	}
}

func TestEnsureSQLiteParentDir_memory(t *testing.T) {
	if err := ensureSQLiteParentDir("file::memory:"); err != nil {
		t.Fatal(err)
	}
}
