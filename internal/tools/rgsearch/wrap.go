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
	return t.inner.Declaration()
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
	args := []string{
		"--json",
		"--max-count", "20",
		"--max-filesize", "2M",
	}
	if !req.ContentCaseSensitive {
		args = append(args, "-i")
	}
	glob := strings.TrimSpace(req.FilePattern)
	if glob != "" && glob != "*" {
		if req.FileCaseSensitive {
			args = append(args, "--glob", glob)
		} else {
			args = append(args, "--iglob", glob)
		}
	}
	args = append(args, "--", req.ContentPattern, ".")
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
	return files, truncated, "ripgrep", true
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
