package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Workspace resolution matches pkg/backend capability/filesystem semantics:
// env ARANEA_WORKSPACE_ROOT or WORKSPACE_ROOT, else walk parents for backend+frontend layout, else cwd.

func workspaceHasDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// WorkspaceRoot returns the absolute sandbox root for native filesystem tools.
func WorkspaceRoot() (string, error) {
	for _, key := range []string{"ARANEA_WORKSPACE_ROOT", "WORKSPACE_ROOT"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return filepath.Abs(filepath.Clean(value))
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	wd, err = filepath.Abs(wd)
	if err != nil {
		return "", err
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if workspaceHasDir(filepath.Join(dir, "backend")) && workspaceHasDir(filepath.Join(dir, "frontend")) {
			return dir, nil
		}
		aranea := filepath.Join(dir, "aranea")
		if workspaceHasDir(filepath.Join(aranea, "backend")) && workspaceHasDir(filepath.Join(aranea, "frontend")) {
			return aranea, nil
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
	}
	return wd, nil
}

// ResolveWorkspacePath maps a user-relative path under WorkspaceRoot or returns an error if outside the sandbox.
func ResolveWorkspacePath(rawPath string) (absPath string, relPath string, err error) {
	root, err := WorkspaceRoot()
	if err != nil {
		return "", "", err
	}
	input := strings.TrimSpace(rawPath)
	if input == "" || input == "." {
		input = "."
	}
	input = filepath.FromSlash(input)
	if !filepath.IsAbs(input) && filepath.Base(root) == "aranea" {
		parts := strings.Split(filepath.Clean(input), string(filepath.Separator))
		if len(parts) > 1 && strings.EqualFold(parts[0], "aranea") {
			input = filepath.Join(parts[1:]...)
		}
	}
	candidate := input
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	candidate, err = filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", "", err
	}
	if rel == "." {
		return candidate, ".", nil
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("path %q is outside workspace sandbox", rawPath)
	}
	return candidate, filepath.ToSlash(rel), nil
}
