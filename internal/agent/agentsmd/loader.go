// Package agentsmd loads the Codex-style AGENTS.md chain for a trusted workspace.
package agentsmd

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultMaxBytes is the default concatenated budget (32 KiB).
const DefaultMaxBytes = 32 * 1024

const (
	overrideName = "AGENTS.override.md"
	agentsName   = "AGENTS.md"
	claudeName   = "CLAUDE.md"
	separator    = "\n\n--- project-doc ---\n\n"
)

// Result is the concatenated project-doc chain.
type Result struct {
	Text      string
	Truncated bool
	Files     []string
}

// IsTrusted reports whether cwd is inside any trusted root after Clean.
// Empty cwd or empty trusted list is untrusted (skip unregistered hosts).
func IsTrusted(cwd string, trustedRoots []string) bool {
	abs, err := filepath.Abs(strings.TrimSpace(cwd))
	if err != nil || abs == "" {
		return false
	}
	abs = filepath.Clean(abs)
	for _, root := range trustedRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		rabs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rabs = filepath.Clean(rabs)
		if abs == rabs || strings.HasPrefix(abs, rabs+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// FindProjectRoot walks cwd→parents for a .git marker, stopping at the
// trusted root. No marker yields cwd (single-directory chain).
func FindProjectRoot(cwd string, trustedRoot string) string {
	abs, err := filepath.Abs(strings.TrimSpace(cwd))
	if err != nil || abs == "" {
		return ""
	}
	abs = filepath.Clean(abs)
	stop, _ := filepath.Abs(strings.TrimSpace(trustedRoot))
	stop = filepath.Clean(stop)
	cur := abs
	for {
		if _, err := os.Stat(filepath.Join(cur, ".git")); err == nil {
			return cur
		}
		if stop != "" && cur == stop {
			return abs
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		if stop != "" && !strings.HasPrefix(cur, stop) {
			return abs
		}
		cur = parent
	}
}

// Load concatenates AGENTS.override.md > AGENTS.md > CLAUDE.md from
// project root down to cwd. Untrusted cwd returns an empty result.
func Load(cwd string, trustedRoots []string, maxBytes int) Result {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	if !IsTrusted(cwd, trustedRoots) {
		return Result{}
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return Result{}
	}
	abs = filepath.Clean(abs)
	trusted := ""
	for _, r := range trustedRoots {
		if IsTrusted(abs, []string{r}) {
			trusted = r
			break
		}
	}
	root := FindProjectRoot(abs, trusted)
	if root == "" {
		return Result{}
	}
	dirs := chainDirs(root, abs)
	var parts []string
	var files []string
	used := 0
	truncated := false
	for _, dir := range dirs {
		path, body, ok := readPreferred(dir)
		if !ok {
			continue
		}
		files = append(files, path)
		if used > 0 {
			if used+len(separator) > maxBytes {
				truncated = true
				break
			}
			parts = append(parts, separator)
			used += len(separator)
		}
		if used+len(body) > maxBytes {
			remain := maxBytes - used
			if remain > 0 {
				parts = append(parts, body[:remain])
			}
			truncated = true
			break
		}
		parts = append(parts, body)
		used += len(body)
	}
	return Result{Text: strings.Join(parts, ""), Truncated: truncated, Files: files}
}

// FormatBlock wraps a non-empty load result for the system prompt.
func FormatBlock(r Result) string {
	body := strings.TrimSpace(r.Text)
	if body == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("<project_agents_md>\n")
	b.WriteString("## Project instructions (AGENTS.md chain)\n")
	b.WriteString("Follow these repository instructions for this workspace. Nested AGENTS.override.md wins over AGENTS.md; later directories override earlier ones.\n\n")
	b.WriteString(body)
	if r.Truncated {
		b.WriteString("\n\n[agents_md_truncated: remaining project docs omitted to stay within the 32KiB budget]\n")
	}
	b.WriteString("\n</project_agents_md>")
	return b.String()
}

func chainDirs(root, cwd string) []string {
	root = filepath.Clean(root)
	cwd = filepath.Clean(cwd)
	rel, err := filepath.Rel(root, cwd)
	if err != nil || strings.HasPrefix(rel, "..") {
		return []string{cwd}
	}
	if rel == "." {
		return []string{root}
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	out := make([]string, 0, len(parts)+1)
	cur := root
	out = append(out, cur)
	for _, p := range parts {
		if p == "" || p == "." {
			continue
		}
		cur = filepath.Join(cur, p)
		out = append(out, cur)
	}
	return out
}

func readPreferred(dir string) (string, string, bool) {
	for _, name := range []string{overrideName, agentsName, claudeName} {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if len(data) == 0 {
			continue
		}
		return path, string(data), true
	}
	return "", "", false
}
