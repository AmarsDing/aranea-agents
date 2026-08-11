package systemprompts

import (
	"embed"
	"io/fs"
	"path"
	"strings"

	"aranea-agents/pkg/apierror"
)

//go:embed prompts/*.md prompts/memory/*.md prompts/skills/*.md prompts/system_admin/*.md prompts/voice_butler/*.md
var embedded embed.FS

// ReadMarkdown returns the body of an embedded prompt file.
// name is relative to prompts/, e.g. "IDENTITY.md", "memory/memory.md".
func ReadMarkdown(name string) (string, error) {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimPrefix(name, "prompts/")
	if name == "" {
		return "", apierror.BadRequest("SYSTEM_PROMPTS", "prompt name is required")
	}
	b, err := embedded.ReadFile(path.Join("prompts", name))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ListTopLevelMarkdown returns *.md file names (without path) under prompts/
// excluding subdirectory entries.
func ListTopLevelMarkdown() ([]string, error) {
	entries, err := fs.ReadDir(embedded, "prompts")
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		out = append(out, e.Name())
	}
	return out, nil
}

// ListSubdirMarkdown returns *.md file names under prompts/<subdir>/.
func ListSubdirMarkdown(subdir string) ([]string, error) {
	subdir = strings.TrimSpace(strings.ReplaceAll(subdir, "\\", "/"))
	if subdir == "" {
		return nil, apierror.BadRequest("SYSTEM_PROMPTS", "subdir is required")
	}
	entries, err := fs.ReadDir(embedded, path.Join("prompts", subdir))
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		out = append(out, e.Name())
	}
	return out, nil
}
