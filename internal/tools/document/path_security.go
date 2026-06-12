package document

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aranea-agents/pkg/apierror"
)

const (
	maxFileSize int64 = 50 * 1024 * 1024 // 50MB
	maxCSVRows        = 100000
)

// ValidatePath cleans and resolves the given path, then verifies it is within
// baseDir (if baseDir is non-empty). It rejects paths that contain ".."
// components after cleaning, UNC paths, and paths that escape baseDir via
// symlinks. Returns the cleaned absolute path on success.
func ValidatePath(path string, baseDir string) (string, error) {
	cleaned := filepath.Clean(path)

	// Reject UNC paths (\\?\, \\server\share) on all platforms.
	if len(cleaned) >= 2 && cleaned[0] == '\\' && cleaned[1] == '\\' {
		return "", apierror.BadRequest(apierror.DomainTool, "UNC paths are not allowed")
	}

	absPath, err := filepath.Abs(cleaned)
	if err != nil {
		return "", apierror.BadRequest(apierror.DomainTool, "resolve absolute path: "+err.Error())
	}

	// Resolve symlinks to prevent traversal via symbolic links.
	// If the path exists, EvalSymlinks must succeed; failure on an existing
	// path is treated as a security error to prevent TOCTOU attacks where an
	// attacker creates a symlink between requests.
	evaluated, err := filepath.EvalSymlinks(absPath)
	if err == nil && evaluated != "" {
		absPath = evaluated
	} else if err != nil {
		// EvalSymlinks fails when the path does not exist (acceptable for
		// new-file writes) but fails on existing paths with permission issues
		// or broken symlinks. Only allow the fallback for non-existent paths.
		if _, statErr := os.Lstat(absPath); statErr == nil {
			// Path exists but EvalSymlinks failed — treat as security error.
			return "", apierror.Internal(apierror.DomainTool, "cannot resolve symlinks for existing path")
		}
		// Path does not exist yet (e.g. write to new file); fall through
		// with the unresolved absPath. The baseDir check below still applies.
	}

	// Reject paths containing ".." components after cleaning.
	for _, component := range strings.Split(absPath, string(filepath.Separator)) {
		if component == ".." {
			return "", apierror.BadRequest(apierror.DomainTool, "path contains '..' component")
		}
	}

	if baseDir != "" {
		cleanBase, err := filepath.Abs(filepath.Clean(baseDir))
		if err != nil {
			return "", apierror.Internal(apierror.DomainTool, "resolve base dir: "+err.Error())
		}
		// Resolve symlinks for base dir too
		evalBase, err := filepath.EvalSymlinks(cleanBase)
		if err == nil && evalBase != "" {
			cleanBase = evalBase
		}
		if !strings.HasSuffix(cleanBase, string(filepath.Separator)) {
			cleanBase += string(filepath.Separator)
		}
		checkPath := absPath
		if !strings.HasSuffix(checkPath, string(filepath.Separator)) {
			checkPath += string(filepath.Separator)
		}
		// Use case-insensitive comparison on Windows for path prefix matching.
		if !pathHasPrefix(checkPath, cleanBase) {
			return "", apierror.Forbidden(apierror.DomainTool, "path is outside base directory")
		}
	}

	return absPath, nil
}

// pathHasPrefix checks if path starts with prefix, using case-insensitive
// comparison on Windows where the filesystem is case-insensitive.
func pathHasPrefix(path, prefix string) bool {
	if filepath.Separator == '\\' {
		return strings.HasPrefix(strings.ToLower(path), strings.ToLower(prefix))
	}
	return strings.HasPrefix(path, prefix)
}

// ValidateFileSize checks that the file at the given path does not exceed
// maxFileSize.
func ValidateFileSize(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return apierror.Internal(apierror.DomainTool, "stat file: "+err.Error())
	}
	if info.Size() > maxFileSize {
		return apierror.BadRequest(apierror.DomainTool, fmt.Sprintf("file size %d exceeds maximum allowed size %d", info.Size(), maxFileSize))
	}
	return nil
}
