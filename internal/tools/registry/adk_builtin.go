package registry

import (
	"fmt"
	"strings"

	"aranea-agents/internal/tools/web_fetch"
	"aranea-agents/internal/tools/web_search"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/exitlooptool"
	"google.golang.org/adk/tool/loadartifactstool"
	"google.golang.org/adk/tool/loadmemorytool"
	"google.golang.org/adk/tool/preloadmemorytool"
)

// AppendADKBuiltin appends one ADK upstream builtin by catalog name when present in enabled.
func AppendADKBuiltin(name string, enabled map[string]bool, out *[]tool.Tool) error {
	name = strings.TrimSpace(name)
	if enabled != nil && !enabled[name] {
		return nil
	}
	switch name {
	case ExitLoop:
		t, err := exitlooptool.New()
		if err != nil {
			return err
		}
		*out = append(*out, t)
		return nil
	case LoadMemory:
		*out = append(*out, loadmemorytool.New())
		return nil
	case PreloadMemory:
		*out = append(*out, preloadmemorytool.New())
		return nil
	case LoadArtifacts:
		*out = append(*out, loadartifactstool.New())
		return nil
	case WebSearch:
		t, err := web_search.New()
		if err != nil {
			return err
		}
		*out = append(*out, t)
		return nil
	case WebFetch:
		t, err := web_fetch.New()
		if err != nil {
			return err
		}
		*out = append(*out, t)
		return nil
	case GoogleSearch:
		// Legacy key: mount the same function tool as web_search (OpenAI path has no Gemini-only google_search).
		t, err := web_search.New()
		if err != nil {
			return err
		}
		*out = append(*out, t)
		return nil
	default:
		return fmt.Errorf("registry: unknown ADK builtin %q", name)
	}
}
