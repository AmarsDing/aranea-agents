package data

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ensureSQLiteParentDir creates the parent directory for file: DSNs when missing.
func ensureSQLiteParentDir(dsn string) error {
	dsn = strings.TrimSpace(dsn)
	if !strings.HasPrefix(dsn, "file:") {
		return nil
	}
	path := strings.TrimPrefix(dsn, "file:")
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	path = strings.TrimSpace(path)
	if path == "" || path == ":memory:" {
		return nil
	}
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("sqlite mkdir %s: %w", dir, err)
	}
	return nil
}
