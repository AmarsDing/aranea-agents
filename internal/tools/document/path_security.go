package document

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

const (
	maxFileSize int64 = 50 * 1024 * 1024 // 50MB
	maxCSVRows        = 100000
)

// ValidatePath cleans and resolves the given path, then verifies it is within
// baseDir (if baseDir is non-empty). It rejects paths that contain ".."
// components after cleaning. Returns the cleaned absolute path on success.
func ValidatePath(path string, baseDir string) (string, error) {
	cleaned := filepath.Clean(path)
	absPath, err := filepath.Abs(cleaned)
	if err != nil {
		return "", kerrors.BadRequest("PATH_SECURITY", "resolve absolute path: "+err.Error())
	}

	// Reject paths containing ".." components after cleaning.
	// filepath.Clean already resolves ".." but a leading ".." that escapes
	// the cwd would still be present as a prefix.
	for _, component := range strings.Split(absPath, string(filepath.Separator)) {
		if component == ".." {
			return "", kerrors.BadRequest("PATH_SECURITY", "path contains '..' component")
		}
	}

	if baseDir != "" {
		cleanBase, err := filepath.Abs(filepath.Clean(baseDir))
		if err != nil {
			return "", kerrors.InternalServer("PATH_SECURITY", "resolve base dir: "+err.Error())
		}
		// Ensure the base dir ends with separator for prefix matching
		// so /data/app does not match /data/app2.
		if !strings.HasSuffix(cleanBase, string(filepath.Separator)) {
			cleanBase += string(filepath.Separator)
		}
		checkPath := absPath
		if !strings.HasSuffix(checkPath, string(filepath.Separator)) {
			checkPath += string(filepath.Separator)
		}
		if !strings.HasPrefix(checkPath, cleanBase) {
			return "", kerrors.Forbidden("PATH_SECURITY", "path is outside base directory")
		}
	}

	return absPath, nil
}

// ValidateFileSize checks that the file at the given path does not exceed
// maxFileSize.
func ValidateFileSize(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return kerrors.InternalServer("PATH_SECURITY", "stat file: "+err.Error())
	}
	if info.Size() > maxFileSize {
		return kerrors.BadRequest("PATH_SECURITY", fmt.Sprintf("file size %d exceeds maximum allowed size %d", info.Size(), maxFileSize))
	}
	return nil
}
