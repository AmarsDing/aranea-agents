// Package reposkills lists SKILL.md files under a trusted workspace
// (.agents/skills or .codex/skills) without importing them into the
// platform catalog.
package reposkills

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/agent/agentsmd"
	"aranea-agents/internal/skill/manifest"
)

const (
	// DefaultMaxEntries caps the cue so a huge repo tree cannot flood the turn.
	DefaultMaxEntries = 24
	defaultDescRunes  = 120
)

// Entry is one repo-local skill discovered on disk.
type Entry struct {
	Slug        string
	Name        string
	Description string
	RelDir      string
	Dir         string
}

// Scan walks the trusted project root and cwd for Codex-style skill trees.
// Untrusted cwd returns nil. Duplicate slugs prefer the cwd tree. This is
// a read-only supplement: it does not write the platform skill DB.
func Scan(cwd string, trustedRoots []string) []Entry {
	if !agentsmd.IsTrusted(cwd, trustedRoots) {
		return nil
	}
	abs, err := filepath.Abs(strings.TrimSpace(cwd))
	if err != nil || abs == "" {
		return nil
	}
	abs = filepath.Clean(abs)
	trusted := ""
	for _, r := range trustedRoots {
		if agentsmd.IsTrusted(abs, []string{r}) {
			trusted = r
			break
		}
	}
	root := agentsmd.FindProjectRoot(abs, trusted)
	if root == "" {
		return nil
	}
	seen := make(map[string]Entry)
	for _, base := range uniqueDirs(root, abs) {
		scanSkillTree(base, ".agents/skills", seen)
		scanSkillTree(base, ".codex/skills", seen)
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]Entry, 0, len(seen))
	for _, e := range seen {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Slug) < strings.ToLower(out[j].Slug)
	})
	if len(out) > DefaultMaxEntries {
		out = out[:DefaultMaxEntries]
	}
	return out
}

// FormatCue renders a trailing dynamic cue. Empty input yields "".
func FormatCue(entries []Entry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<workspace_skills>\n")
	b.WriteString("## Workspace skills (repo FS)\n")
	b.WriteString("These SKILL.md files live under .agents/skills or .codex/skills. ")
	b.WriteString("They are not the platform catalog. Load with $slug or skill_load.\n\n")
	for _, e := range entries {
		slug := strings.TrimSpace(e.Slug)
		if slug == "" {
			continue
		}
		desc := strings.TrimSpace(e.Description)
		if desc == "" {
			desc = strings.TrimSpace(e.Name)
		}
		if desc != "" {
			b.WriteString("- $")
			b.WriteString(slug)
			b.WriteString(" — ")
			b.WriteString(desc)
			b.WriteByte('\n')
		} else {
			b.WriteString("- $")
			b.WriteString(slug)
			b.WriteByte('\n')
		}
	}
	b.WriteString("</workspace_skills>")
	return b.String()
}

func uniqueDirs(paths ...string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = filepath.Clean(strings.TrimSpace(p))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func scanSkillTree(base, rel string, seen map[string]Entry) {
	dir := filepath.Join(base, filepath.FromSlash(rel))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, d := range entries {
		if !d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			continue
		}
		slug := strings.TrimSpace(d.Name())
		if slug == "" {
			continue
		}
		body, ok := readSkillMD(filepath.Join(dir, d.Name()))
		if !ok {
			continue
		}
		m := manifest.Parse(body)
		desc := strings.TrimSpace(m.Description)
		if utf8.RuneCountInString(desc) > defaultDescRunes {
			desc = string([]rune(desc)[:defaultDescRunes])
		}
		name := strings.TrimSpace(m.Name)
		if name == "" {
			name = slug
		}
		absDir := filepath.Join(dir, d.Name())
		seen[strings.ToLower(slug)] = Entry{
			Slug:        slug,
			Name:        name,
			Description: desc,
			RelDir:      filepath.ToSlash(filepath.Join(rel, d.Name())),
			Dir:         absDir,
		}
	}
}

// Lookup finds an entry by slug or display name (case-insensitive).
func Lookup(entries []Entry, name string) (Entry, bool) {
	want := strings.ToLower(strings.TrimSpace(name))
	if want == "" {
		return Entry{}, false
	}
	for _, e := range entries {
		if strings.ToLower(e.Slug) == want || strings.ToLower(e.Name) == want {
			return e, true
		}
	}
	return Entry{}, false
}

// Slugs returns the directory names in scan order.
func Slugs(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if s := strings.TrimSpace(e.Slug); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func readSkillMD(dir string) (string, bool) {
	for _, name := range []string{"SKILL.md", "skill.md"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || len(data) == 0 {
			continue
		}
		return string(data), true
	}
	return "", false
}
