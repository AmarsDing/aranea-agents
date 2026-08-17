package testexec

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/tools"
	webresearchpkg "aranea-agents/internal/tools/webresearch"
	"aranea-agents/pkg/loggateway"

	"aranea-agents/pkg/apierror"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const (
	DefaultTimeoutSec = 30
	MaxTimeoutSec     = 120
)

// Result is the outcome of a single catalog tool test invocation.
type Result struct {
	Status        string
	ResultPreview string
	ErrorMessage  string
	DurationMS    int
}

// CatalogTool is the minimal tool row needed for a test invocation.
type CatalogTool struct {
	Key               string
	Source            string
	ConfigJSON        string
	DefaultConfigJSON string
	MetadataJSON      string
}

// Execute runs one tool call for catalog / OpenAPI-backed tools (admin test harness).
// platform supplies system_settings for web_research when agent config has no API key.
func Execute(ctx context.Context, tool CatalogTool, argumentsJSON string, timeoutSec int, platform *webresearchpkg.PlatformFields, lg loggateway.Logger) (Result, error) {
	key := strings.TrimSpace(tool.Key)
	if key == "" {
		return Result{}, apierror.BadRequest(apierror.DomainTool, "tool key is required")
	}
	src := strings.ToLower(strings.TrimSpace(tool.Source))
	if src == "mcp" {
		return Result{}, apierror.BadRequest(apierror.DomainTool, "MCP tools must be tested via MCPServer TestMCPServer")
	}
	if key == toolKeyKnowledgeSearch || key == toolKeyCallAgent {
		return Result{}, apierror.BadRequest(apierror.DomainTool, fmt.Sprintf("tool %q requires a live agent session to test", key))
	}

	args := normalizeArgsJSON(argumentsJSON)
	timeout := clampTimeout(timeoutSec)
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	started := time.Now().UTC()
	merged := mergeConfigJSON(tool.ConfigJSON, tool.DefaultConfigJSON)
	asmCfg, ok, asmErr := AssemblyForCatalogKey(key, merged, platform, lg)
	if asmErr != nil {
		return Result{}, apierror.Internal(apierror.DomainTool, fmt.Sprintf("tool %q assembly config error: %s", key, asmErr.Error()))
	}
	if !ok {
		if spec, ok := openAPISpecFromCatalogTool(tool); ok {
			asmCfg = tools.AssemblyConfig{
				EnabledTools: []string{"openapi"},
				OpenAPISpecs: []tools.OpenAPISpecConfig{spec},
				Lg:           lg,
			}
		} else {
			return Result{}, apierror.BadRequest(apierror.DomainTool, fmt.Sprintf("tool %q is not supported for online test yet", key))
		}
	}

	ts, err := tools.Assemble(runCtx, asmCfg)
	if err != nil {
		return failResult(started, err), nil
	}
	tools.ApplyDefaultDecorators(ts, lg)
	names := catalogToolNames(key)
	if key == "web_research" {
		names = append([]string{"web_research"}, names...)
	}
	callable, err := findCallable(runCtx, ts, names...)
	if err != nil {
		return failResult(started, err), nil
	}
	out, callErr := callable.Call(runCtx, []byte(args))
	duration := int(time.Since(started).Milliseconds())
	if callErr != nil {
		return Result{
			Status:        "error",
			ErrorMessage:  truncate(callErr.Error(), 2000),
			DurationMS:    duration,
			ResultPreview: previewValue(out),
		}, nil
	}
	return Result{
		Status:        "success",
		ResultPreview: previewValue(out),
		DurationMS:    duration,
	}, nil
}

func clampTimeout(sec int) int {
	if sec <= 0 {
		return DefaultTimeoutSec
	}
	if sec > MaxTimeoutSec {
		return MaxTimeoutSec
	}
	return sec
}

func normalizeArgsJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return "{}"
	}
	var probe any
	if json.Unmarshal([]byte(raw), &probe) != nil {
		return "{}"
	}
	return raw
}

func failResult(started time.Time, err error) Result {
	return Result{
		Status:       "error",
		ErrorMessage: truncate(err.Error(), 2000),
		DurationMS:   int(time.Since(started).Milliseconds()),
	}
}

func catalogToolNames(key string) []string {
	switch key {
	case "shell_exec":
		return []string{"shell_exec", "exec_command"}
	case "arxiv_search":
		return []string{"search", "arxiv_search"}
	case "google_search":
		return []string{"search", "google_search"}
	case "working_memory.read":
		return []string{"read"}
	case "working_memory.list":
		return []string{"list"}
	case "working_memory.write":
		return []string{"write"}
	case "working_memory.patch":
		return []string{"patch"}
	case "working_memory.delete":
		return []string{"delete"}
	default:
		return []string{key}
	}
}

func findCallable(ctx context.Context, ts *tools.AssembledToolsets, names ...string) (trpctool.CallableTool, error) {
	if ts == nil {
		return nil, apierror.Internal(apierror.DomainTool, "no toolsets assembled")
	}
	for _, name := range names {
		if t, ok := matchCallable(ts.Tools, name); ok {
			return t, nil
		}
		for _, set := range ts.ToolSets {
			if set == nil {
				continue
			}
			if t, ok := matchCallable(set.Tools(ctx), name); ok {
				return t, nil
			}
		}
	}
	return nil, apierror.NotFound(apierror.DomainTool, fmt.Sprintf("callable tool not found in assembly (tried %v)", names))
}

func matchCallable(toolsList []trpctool.Tool, name string) (trpctool.CallableTool, bool) {
	for _, t := range toolsList {
		if t == nil {
			continue
		}
		d := t.Declaration()
		if d == nil || strings.TrimSpace(d.Name) != name {
			continue
		}
		if c, ok := t.(trpctool.CallableTool); ok {
			return c, true
		}
	}
	return nil, false
}

func previewValue(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		s := fmt.Sprint(v)
		return truncate(s, 4000)
	}
	return truncate(string(b), 4000)
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

type openAPIMeta struct {
	OpenAPISpecURL  string `json:"openapi_spec_url"`
	OpenAPISpecData string `json:"openapi_spec_data"`
	SpecURL         string `json:"spec_url"`
	SpecData        string `json:"spec_data"`
}

func mergeConfigJSON(baseJSON, defaultJSON string) map[string]any {
	out := map[string]any{}
	mergeJSONInto(out, defaultJSON)
	mergeJSONInto(out, baseJSON)
	return out
}

func mergeJSONInto(dst map[string]any, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return
	}
	var patch map[string]any
	if json.Unmarshal([]byte(raw), &patch) != nil {
		return
	}
	for k, v := range patch {
		dst[k] = v
	}
}

func openAPISpecFromCatalogTool(tool CatalogTool) (tools.OpenAPISpecConfig, bool) {
	raw := strings.TrimSpace(tool.MetadataJSON)
	if raw == "" || raw == "{}" {
		return tools.OpenAPISpecConfig{}, false
	}
	var m openAPIMeta
	if json.Unmarshal([]byte(raw), &m) != nil {
		return tools.OpenAPISpecConfig{}, false
	}
	url := strings.TrimSpace(m.OpenAPISpecURL)
	if url == "" {
		url = strings.TrimSpace(m.SpecURL)
	}
	data := strings.TrimSpace(m.OpenAPISpecData)
	if data == "" {
		data = strings.TrimSpace(m.SpecData)
	}
	if url == "" && data == "" {
		return tools.OpenAPISpecConfig{}, false
	}
	return tools.OpenAPISpecConfig{
		Name:     strings.TrimSpace(tool.Key),
		SpecURL:  url,
		SpecData: []byte(data),
	}, true
}
