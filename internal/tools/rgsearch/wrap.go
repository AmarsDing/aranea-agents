package rgsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Runner executes ripgrep. Tests inject a fake; production uses execRipgrep.
type Runner func(ctx context.Context, dir string, args []string) (stdout string, err error)

type fileToolSetWrap struct {
	inner   trpctool.ToolSet
	baseDir string
	lg      loggateway.Logger
	run     Runner
}

// WrapToolSet intercepts search_content: prefer ripgrep, cap result size, fall
// back to the inner Go scanner when rg is missing or the path is a virtual ref.
func WrapToolSet(inner trpctool.ToolSet, baseDir string, lg loggateway.Logger) trpctool.ToolSet {
	return wrapToolSet(inner, baseDir, lg, nil)
}

func wrapToolSet(inner trpctool.ToolSet, baseDir string, lg loggateway.Logger, run Runner) trpctool.ToolSet {
	if inner == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	if run == nil {
		run = execRipgrep
	}
	return &fileToolSetWrap{inner: inner, baseDir: baseDir, lg: lg, run: run}
}

func (s *fileToolSetWrap) Name() string {
	if s.inner == nil {
		return ""
	}
	return s.inner.Name()
}

func (s *fileToolSetWrap) Close() error {
	if s.inner == nil {
		return nil
	}
	return s.inner.Close()
}

func (s *fileToolSetWrap) Tools(ctx context.Context) []trpctool.Tool {
	if s.inner == nil {
		return nil
	}
	raw := s.inner.Tools(ctx)
	out := make([]trpctool.Tool, len(raw))
	for i, t := range raw {
		ct, ok := t.(trpctool.CallableTool)
		if !ok {
			out[i] = t
			continue
		}
		name := ""
		if d := ct.Declaration(); d != nil {
			name = d.Name
		}
		if name != "search_content" {
			out[i] = t
			continue
		}
		out[i] = &searchContentTool{inner: ct, wrap: s}
	}
	return out
}

type searchContentTool struct {
	inner trpctool.CallableTool
	wrap  *fileToolSetWrap
}

func (t *searchContentTool) Declaration() *trpctool.Declaration {
	if t.inner == nil {
		return nil
	}
	d := t.inner.Declaration()
	if d == nil {
		return nil
	}
	cp := *d
	schema := &trpctool.Schema{Type: "object", Properties: map[string]*trpctool.Schema{}}
	if d.InputSchema != nil {
		cloned := *d.InputSchema
		props := map[string]*trpctool.Schema{}
		for k, v := range d.InputSchema.Properties {
			props[k] = v
		}
		cloned.Properties = props
		schema = &cloned
	}
	addSearchContentSchema(schema)
	cp.InputSchema = schema
	if cp.Description != "" && !strings.Contains(cp.Description, "head_limit") {
		cp.Description += " Optional: after/before/context line windows, type (rg --type), multiline, head_limit/offset pagination."
	}
	return &cp
}

func (t *searchContentTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if t == nil || t.inner == nil || t.wrap == nil {
		return nil, errors.New("search_content not initialized")
	}
	var req searchRequest
	if err := json.Unmarshal(jsonArgs, &req); err != nil {
		return t.inner.Call(ctx, jsonArgs)
	}
	if strings.TrimSpace(req.ContentPattern) == "" {
		return t.inner.Call(ctx, jsonArgs)
	}
	if isVirtualRef(req.Path) || isVirtualRef(req.FilePattern) {
		return t.capInner(ctx, jsonArgs)
	}
	matches, truncated, engine, ok := t.wrap.searchRipgrep(ctx, req)
	if !ok {
		return t.capInner(ctx, jsonArgs)
	}
	msg := "Found " + strconv.Itoa(len(matches)) + " files matching"
	if truncated {
		msg += " (truncated; narrow path or file_pattern)"
	}
	return searchResponse{
		BaseDirectory:  t.wrap.baseDir,
		Path:           req.Path,
		FilePattern:    req.FilePattern,
		ContentPattern: req.ContentPattern,
		FileMatches:    matches,
		Message:        msg,
		Engine:         engine,
		Truncated:      truncated,
	}, nil
}

func (t *searchContentTool) capInner(ctx context.Context, jsonArgs []byte) (any, error) {
	out, err := t.inner.Call(ctx, jsonArgs)
	if err != nil {
		return out, err
	}
	return capAnyResult(out), nil
}

func (s *fileToolSetWrap) searchRipgrep(ctx context.Context, req searchRequest) ([]*fileMatch, bool, string, bool) {
	if s.run == nil {
		return nil, false, "", false
	}
	searchRoot := strings.TrimSpace(req.Path)
	if searchRoot == "" {
		searchRoot = "."
	}
	absRoot := searchRoot
	if s.baseDir != "" && !filepath.IsAbs(searchRoot) {
		absRoot = filepath.Join(s.baseDir, searchRoot)
	}
	args := append(s.ripgrepArgs(req), "--", req.ContentPattern, ".")
	dir := absRoot
	if st, err := os.Stat(absRoot); err != nil || !st.IsDir() {
		dir = s.baseDir
		if dir == "" {
			dir = "."
		}
		args[len(args)-1] = searchRoot
	}
	stdout, err := s.run(ctx, dir, args)
	if err != nil && !isNoMatch(err) {
		s.lg.Debug("ripgrep unavailable, falling back to Go search",
			loggateway.StepID("tool.search.rg_fallback"),
			loggateway.Err(err))
		return nil, false, "", false
	}
	files, truncated := parseRipgrepJSON(stdout, dir, "")
	paged, pageTrunc := paginateFileMatches(files, req.Offset, req.HeadLimit)
	return paged, truncated || pageTrunc, "ripgrep", true
}

func (s *fileToolSetWrap) ripgrepArgs(req searchRequest) []string {
	args := []string{
		"--json",
		"--max-count", "20",
		"--max-filesize", "2M",
	}
	if !req.ContentCaseSensitive {
		args = append(args, "-i")
	}
	if ctx := clampContext(req.Context); ctx > 0 {
		args = append(args, "-C", strconv.Itoa(ctx))
	} else {
		if n := clampContext(req.After); n > 0 {
			args = append(args, "-A", strconv.Itoa(n))
		}
		if n := clampContext(req.Before); n > 0 {
			args = append(args, "-B", strconv.Itoa(n))
		}
	}
	if req.Multiline {
		args = append(args, "-U", "--multiline-dotall")
	}
	if typ := sanitizeRGType(req.Type); typ != "" {
		args = append(args, "--type", typ)
	}
	glob := strings.TrimSpace(req.FilePattern)
	if glob != "" && glob != "*" {
		if req.FileCaseSensitive {
			args = append(args, "--glob", glob)
		} else {
			args = append(args, "--iglob", glob)
		}
	}
	return args
}

func addSearchContentSchema(schema *trpctool.Schema) {
	if schema == nil {
		return
	}
	if schema.Properties == nil {
		schema.Properties = map[string]*trpctool.Schema{}
	}
	put := func(name, typ, desc string) {
		if _, ok := schema.Properties[name]; ok {
			return
		}
		schema.Properties[name] = &trpctool.Schema{Type: typ, Description: desc}
	}
	put("after", "integer", "Lines after each match (rg -A). Alias: -A. Max 10.")
	put("before", "integer", "Lines before each match (rg -B). Alias: -B. Max 10.")
	put("context", "integer", "Lines before and after each match (rg -C). Alias: -C. Max 10.")
	put("type", "string", "ripgrep --type name (e.g. go, py, rust). Alphanumeric only.")
	put("multiline", "boolean", "Enable ripgrep multiline mode (-U --multiline-dotall).")
	put("head_limit", "integer", "Keep at most this many match lines after offset (pagination).")
	put("offset", "integer", "Skip this many match lines before applying head_limit.")
}

func clampContext(n int) int {
	if n <= 0 {
		return 0
	}
	if n > 10 {
		return 10
	}
	return n
}

func sanitizeRGType(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || len(s) > 32 {
		return ""
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '+' || r == '-' {
			continue
		}
		return ""
	}
	return s
}

func execRipgrep(ctx context.Context, dir string, args []string) (string, error) {
	if _, err := exec.LookPath("rg"); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "rg", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil && !isNoMatch(err) {
		if stderr.Len() > 0 {
			return stdout.String(), errors.New(strings.TrimSpace(stderr.String()))
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

func isNoMatch(err error) bool {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode() == 1
	}
	return false
}

func isVirtualRef(s string) bool {
	s = strings.TrimSpace(s)
	return strings.Contains(s, "://")
}

func capAnyResult(out any) any {
	if out == nil {
		return out
	}
	b, err := json.Marshal(out)
	if err != nil {
		return out
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return out
	}
	raw, ok := m["file_matches"]
	if !ok {
		return out
	}
	fb, err := json.Marshal(raw)
	if err != nil {
		return out
	}
	var files []*fileMatch
	if err := json.Unmarshal(fb, &files); err != nil {
		return out
	}
	capped, trunc := capFileMatches(files)
	m["file_matches"] = capped
	if trunc {
		m["truncated"] = true
		if _, ok := m["engine"]; !ok {
			m["engine"] = "go"
		}
	}
	return m
}
