package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// DefaultPath returns the platform-appropriate config file path.
//
//	Linux/macOS : $XDG_CONFIG_HOME/aranea/config.toml  (falls back to ~/.config)
//	Windows     : %APPDATA%\aranea\config.toml
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user config directory: %w", err)
	}
	return filepath.Join(dir, "aranea", "config.toml"), nil
}

// CacheDir returns the platform-appropriate CLI cache directory.
//
//	Linux  : ~/.cache/aranea
//	macOS  : ~/Library/Caches/aranea
//	Windows: %LOCALAPPDATA%\aranea\Cache
func CacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user cache directory: %w", err)
	}
	return filepath.Join(dir, "aranea"), nil
}

// LogsDir returns the log directory path.
func LogsDir() (string, error) {
	cache, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "logs"), nil
}

// TmpDir returns the temporary directory path used by skill install.
func TmpDir() (string, error) {
	cache, err := CacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "tmp"), nil
}

// EnsureSecurePerm checks that the file's permissions are ≤0600.
// On Windows the check is skipped (always returns nil).
// If permissions are too open, a *CLIError with Code=INSECURE_CONFIG_PERM is returned.
func EnsureSecurePerm(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	perm := fi.Mode().Perm()
	if perm > 0o600 {
		return &insecurePermError{path: path, perm: perm}
	}
	return nil
}

// FixPerm sets the file permissions to 0600 (no-op on Windows).
func FixPerm(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	return os.Chmod(path, 0o600)
}

// insecurePermError is returned when a config file has permissions > 0600.
type insecurePermError struct {
	path string
	perm fs.FileMode
}

func (e *insecurePermError) Error() string {
	return fmt.Sprintf(
		"INSECURE_CONFIG_PERM: %s has permissions %04o (expected ≤0600); run: chmod 600 %s",
		e.path, e.perm, e.path,
	)
}
