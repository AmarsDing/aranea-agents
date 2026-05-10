// Package workspace_search 提供工作区字面/正则搜索（rg 优先，WalkDir 回退）。
package workspace_search

import (
	"path/filepath"
	"strings"
)

// DefaultSkippedPathSegments lists directory/file name segments to skip (WalkDir + rg glob exclude).
// Keep in sync with docs/design/agent-repo-retrieval-context-engineering.md §7.4.
var DefaultSkippedPathSegments = []string{
	".git", "node_modules", "vendor", "dist", "build", ".cursor",
}

// ShouldSkipPath returns true if any path element equals a skipped segment (case-sensitive on all OS for consistency).
func ShouldSkipPath(rel string) bool {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" || rel == "." {
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		for _, seg := range DefaultSkippedPathSegments {
			if part == seg {
				return true
			}
		}
	}
	return false
}
