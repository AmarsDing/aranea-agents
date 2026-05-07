package skillruntime

import (
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

// NewEnabledSkillsRootFS exposes only top-level slug subdirectories listed in allowedSlugs
// (each slug maps to {root}/{slug}/... on the real filesystem).
func NewEnabledSkillsRootFS(root string, allowedSlugs []string) fs.FS {
	m := map[string]struct{}{}
	for _, s := range allowedSlugs {
		s = strings.TrimSpace(s)
		if s == "" || strings.ContainsAny(s, `/\`) || strings.Contains(s, "..") {
			continue
		}
		m[s] = struct{}{}
	}
	return &enabledSkillsRootFS{root: filepath.Clean(root), allow: m}
}

type enabledSkillsRootFS struct {
	root  string
	allow map[string]struct{}
}

func (e *enabledSkillsRootFS) Open(name string) (fs.File, error) {
	name = pathpkg.Clean("/" + filepath.ToSlash(strings.TrimSpace(name)))
	name = strings.TrimPrefix(name, "/")
	if name == "" || name == "." {
		return &enabledRootDir{fs: e}, nil
	}
	parts := strings.Split(name, "/")
	top := parts[0]
	if _, ok := e.allow[top]; !ok {
		return nil, fs.ErrNotExist
	}
	full := filepath.Join(append([]string{e.root}, parts...)...)
	full = filepath.Clean(full)
	rel, err := filepath.Rel(e.root, full)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return nil, fs.ErrNotExist
	}
	return os.Open(full)
}

type enabledRootDir struct {
	fs *enabledSkillsRootFS
}

func (d *enabledRootDir) Stat() (fs.FileInfo, error) { return os.Stat(d.fs.root) }
func (d *enabledRootDir) Read([]byte) (int, error)   { return 0, fs.ErrInvalid }
func (d *enabledRootDir) Close() error               { return nil }

func (d *enabledRootDir) ReadDir(n int) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(d.fs.root)
	if err != nil {
		return nil, err
	}
	var out []fs.DirEntry
	for _, ent := range entries {
		if !ent.IsDir() || strings.HasPrefix(ent.Name(), ".") {
			continue
		}
		if _, ok := d.fs.allow[ent.Name()]; ok {
			out = append(out, ent)
		}
	}
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out, nil
}
