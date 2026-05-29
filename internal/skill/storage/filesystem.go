package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"aranea-agents/internal/biz/skill"
	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type skillFilesystem struct {
	resolveRootFn func(ctx context.Context) string
}

func NewSkillFilesystem(resolveRootFn func(ctx context.Context) string) skill.SkillFilesystem {
	if resolveRootFn == nil {
		resolveRootFn = func(_ context.Context) string { return ResolveRoot() }
	}
	return &skillFilesystem{resolveRootFn: resolveRootFn}
}

func (f *skillFilesystem) ResolveRoot(ctx context.Context) string {
	return f.resolveRootFn(ctx)
}

func (f *skillFilesystem) CreateSkillDir(slug string, body string) (string, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return "", kerrors.BadRequest("SKILL", "slug is required for skill directory creation")
	}
	if strings.Contains(slug, "..") || strings.HasPrefix(slug, "/") {
		return "", kerrors.BadRequest("SKILL", "slug contains unsafe path characters")
	}
	root := f.resolveRootFn(context.Background())
	dir := filepath.Join(root, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		return "", err
	}
	return dir, nil
}

func (f *skillFilesystem) ListFiles(dir string) ([]skill.SkillFileEntry, error) {
	var items []skill.SkillFileEntry
	walkErr := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		items = append(items, skill.SkillFileEntry{
			Path:      rel,
			Name:      pathBase(rel),
			Language:  languageForPath(rel),
			Size:      info.Size(),
			UpdatedAt: info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return items, nil
}

func (f *skillFilesystem) ReadFile(dir string, relPath string) (skill.SkillFileContent, error) {
	root, path, err := f.SafeFilePath(dir, relPath)
	if err != nil {
		return skill.SkillFileContent{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return skill.SkillFileContent{}, err
	}
	if info.IsDir() {
		return skill.SkillFileContent{}, errors.New("skill file path points to a directory")
	}
	if info.Size() > 2*1024*1024 {
		return skill.SkillFileContent{}, errors.New("skill file is too large to edit")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return skill.SkillFileContent{}, err
	}
	rel, _ := filepath.Rel(root, path)
	rel = filepath.ToSlash(rel)
	return skill.SkillFileContent{Path: rel, Content: string(raw), Language: languageForPath(rel)}, nil
}

func (f *skillFilesystem) WriteFile(dir string, relPath string, content string) error {
	_, path, err := f.SafeFilePath(dir, relPath)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func (f *skillFilesystem) DeleteFile(dir string, relPath string) error {
	_, path, err := f.SafeFilePath(dir, relPath)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func (f *skillFilesystem) RootAccessible(ctx context.Context) bool {
	root := f.resolveRootFn(ctx)
	st, err := os.Stat(root)
	return err == nil && st.IsDir()
}

func (f *skillFilesystem) DirExists(dir string) bool {
	st, err := os.Stat(dir)
	return err == nil && st.IsDir()
}

func (f *skillFilesystem) SafeFilePath(dir string, relPath string) (string, string, error) {
	relPath = strings.TrimSpace(filepath.ToSlash(relPath))
	if relPath == "" || strings.Contains(relPath, "..") || strings.HasPrefix(relPath, "/") {
		return "", "", errors.New("unsafe skill file path")
	}
	path := filepath.Join(dir, filepath.FromSlash(relPath))
	absRoot, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	if absPath != absRoot && !strings.HasPrefix(absPath, absRoot+string(os.PathSeparator)) {
		return "", "", errors.New("skill file path escapes skill directory")
	}
	return absRoot, absPath, nil
}

func languageForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return "markdown"
	case ".js", ".mjs", ".cjs":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".py":
		return "python"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".sh":
		return "shell"
	default:
		return "text"
	}
}

func pathBase(name string) string {
	name = strings.Trim(filepath.ToSlash(name), "/")
	if name == "" || name == "." {
		return ""
	}
	parts := strings.Split(name, "/")
	return parts[len(parts)-1]
}
