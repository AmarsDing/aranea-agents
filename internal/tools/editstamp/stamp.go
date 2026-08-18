package editstamp

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	relFile  = ".aranea/edited-paths.txt"
	maxPaths = 32
)

var (
	mu sync.Mutex
)

// Record appends a workspace-relative path to the recent-edit stamp.
func Record(baseDir, filePath string) {
	p := normalize(filePath)
	if p == "" || strings.HasPrefix(p, ".aranea/") {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	dir := strings.TrimSpace(baseDir)
	if dir == "" {
		dir = "."
	}
	stampDir := filepath.Join(dir, ".aranea")
	if err := os.MkdirAll(stampDir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dir, filepath.FromSlash(relFile))
	cur := readLocked(path)
	out := make([]string, 0, len(cur)+1)
	for _, e := range cur {
		if e != p {
			out = append(out, e)
		}
	}
	out = append(out, p)
	if len(out) > maxPaths {
		out = out[len(out)-maxPaths:]
	}
	_ = os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o644)
}

// List returns recently edited workspace-relative paths (oldest first).
func List(baseDir string) []string {
	mu.Lock()
	defer mu.Unlock()
	dir := strings.TrimSpace(baseDir)
	if dir == "" {
		dir = "."
	}
	return readLocked(filepath.Join(dir, filepath.FromSlash(relFile)))
}

func readLocked(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	seen := map[string]struct{}{}
	for _, line := range strings.Split(string(b), "\n") {
		line = normalize(line)
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		out = append(out, line)
	}
	return out
}

func normalize(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || strings.Contains(p, "://") {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	return strings.Trim(p, "/")
}
