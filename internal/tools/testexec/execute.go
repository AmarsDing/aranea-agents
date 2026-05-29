package testexec

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/tools"
	webresearchpkg "aranea-agents/internal/tools/webresearch"

	kerrors "github.com/go-kratos/kratos/v2/errors"

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
func Execute(ctx context.Context, tool CatalogTool, argumentsJSON string, timeoutSec int, platform *webresearchpkg.PlatformFields) (Result, error) {
	key := strings.TrimSpace(tool.Key)
	if key == "" {
		return Result{}, kerrors.BadRequest("TOOL", "tool key is required")
	}
	src := strings.ToLower(strings.TrimSpace(tool.Source))
	if src == "mcp" {
		return Result{}, kerrors.BadRequest("TOOL", "MCP tools must be tested via MCPServer TestMCPServer")
	}
	if key == toolKeyKnowledgeSearch || key == toolKeyCallAgent {
		return Result{}, kerrors.BadRequest("TOOL", fmt.Sprintf("tool %q requires a live agent session to test", key))
	}

	args := normalizeArgsJSON(argumentsJSON)
	timeout := clampTimeout(timeoutSec)
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	started := time.Now().UTC()
	merged := mergeConfigJSON(tool.ConfigJSON, tool.DefaultConfigJSON)
	asmCfg, ok := AssemblyForCatalogKey(key, merged, platform)
	if !ok {
		if spec, ok := openAPISpecFromCatalogTool(tool); ok {
			asmCfg = tools.AssemblyConfig{
				EnabledTools: []string{"openapi"},
				OpenAPISpecs: []tools.OpenAPISpecConfig{spec},
			}
		} else {
			return Result{}, kerrors.BadRequest("TOOL", fmt.Sprintf("tool %q is not supported for online test yet", key))
		}
	}

	ts, err := tools.Assemble(runCtx, asmCfg)
	if err != nil {
		return failResult(started, err), nil
	}
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
	if key == "shell_exec" {
		return []string{"shell_exec", "exec_command"}
	}
	return []string{key}
}

func findCallable(ctx context.Context, ts *tools.AssembledToolsets, names ...string) (trpctool.CallableTool, error) {
	if ts == nil {
		return nil, kerrors.InternalServer("TOOL", "no toolsets assembled")
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
	return nil, kerrors.NotFound("TOOL", fmt.Sprintf("callable tool not found in assembly (tried %v)", names))
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
	if len(s) <= n {
		return s
	}
	return s[:n]
}

type openAPIMeta struct {
	OpenAPISpecURL  string `json:"openapi_spec_url"`
	OpenAPISpecData string `json:"openapi_spec_data"`
	SpecURL         string `json:"spec_url"`
	SpecData        string `json:"spec_data"`
}

func mergeConfigJSON(baseJSON, defaultJSON string) map[string]any {
	out := map[string]any{}
	mergeJSONInto(out, baseJSON)
	mergeJSONInto(out, defaultJSON)
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
