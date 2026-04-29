package util

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func ResolveSkillStorageRoot() string {
	if value := strings.TrimSpace(os.Getenv("SKILL_ROOT")); value != "" {
		return AbsoluteStoragePath(value)
	}
	if value := strings.TrimSpace(os.Getenv("SKILL_STORAGE_ROOT")); value != "" {
		return AbsoluteStoragePath(value)
	}
	return AbsoluteStoragePath(DefaultSkillStorageRoot(runtime.GOOS))
}

func DefaultSkillStorageRoot(goos string) string {
	if dir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(dir) != "" {
		return filepath.Join(dir, "Aranea", "skills")
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		switch goos {
		case "windows":
			return filepath.Join(home, "AppData", "Roaming", "Aranea", "skills")
		case "darwin", "ios":
			return filepath.Join(home, "Library", "Application Support", "Aranea", "skills")
		default:
			return filepath.Join(home, ".config", "aranea", "skills")
		}
	}
	return filepath.Join("data", "skills")
}

func AbsoluteStoragePath(root string) string {
	root = strings.TrimSpace(root)
	if root == "~" || strings.HasPrefix(root, "~"+string(os.PathSeparator)) || strings.HasPrefix(root, "~/") {
		if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
			root = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(root, "~"+string(os.PathSeparator)), "~/"))
		}
	}
	if abs, err := filepath.Abs(root); err == nil {
		return abs
	}
	return root
}
