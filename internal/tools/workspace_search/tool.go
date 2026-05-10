package workspace_search

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/tools/argmap"
	"aranea-agents/internal/tools/specs"
	"aranea-agents/internal/tools/workspace"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

const (
	defaultMaxResults       = 50
	hardMaxResults          = 500
	defaultRipgrepTimeout   = 30 * time.Second
	maxPatternRunes         = 512
	defaultMaxFileSizeBytes = 2 * 1024 * 1024
)

type args struct {
	Query            string   `json:"query"`
	Mode             string   `json:"mode,omitempty"`
	PathPrefix       string   `json:"path_prefix,omitempty"`
	Glob             string   `json:"glob,omitempty"`
	MaxResults       int      `json:"max_results,omitempty"`
	MaxMatchesPerFile int     `json:"max_matches_per_file,omitempty"`
	ContextLines     int      `json:"context_lines,omitempty"`
}

const desc = `Search text under the workspace root. Prefer this over list_files when looking for symbols or strings. Use mode "substring" (default) for literals or "regex" for patterns. Results are capped: narrow with path_prefix or glob. If you already know the exact file path, use read_file instead of searching.`

// Match is one hit in the workspace.
type Match struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Snippet  string `json:"snippet"`
}

// Run executes the search (also used by legacy OpenAI tool loop).
func Run(ctx context.Context, argsMap map[string]any) (map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	q := strings.TrimSpace(argmap.String(argsMap, "query"))
	if q == "" {
		return nil, errors.New("query is required")
	}
	if utf8.RuneCountInString(q) > maxPatternRunes {
		return nil, fmt.Errorf("query too long (max %d runes)", maxPatternRunes)
	}
	mode := strings.ToLower(strings.TrimSpace(argmap.String(argsMap, "mode")))
	if mode == "" {
		mode = "substring"
	}
	if mode != "substring" && mode != "regex" {
		return nil, fmt.Errorf("mode must be substring or regex, got %q", mode)
	}
	maxResults := intFromArg(argsMap, "max_results", defaultMaxResults)
	if maxResults <= 0 {
		maxResults = defaultMaxResults
	}
	if maxResults > hardMaxResults {
		maxResults = hardMaxResults
	}
	maxPerFile := intFromArg(argsMap, "max_matches_per_file", 0)
	if maxPerFile < 0 {
		maxPerFile = 0
	}
	ctxLines := intFromArg(argsMap, "context_lines", 0)
	if ctxLines < 0 || ctxLines > 5 {
		ctxLines = 0
	}
	pathPrefix := strings.TrimSpace(argmap.String(argsMap, "path_prefix"))
	globPat := strings.TrimSpace(argmap.String(argsMap, "glob"))

	root, err := workspace.Root()
	if err != nil {
		return nil, err
	}
	var searchAbs string
	if pathPrefix != "" {
		var rel string
		searchAbs, rel, err = workspace.ResolvePath(pathPrefix)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(searchAbs)
		if err != nil {
			return nil, err
		}
		if ShouldSkipPath(rel) {
			return nil, fmt.Errorf("path_prefix %q is under a skipped directory", pathPrefix)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("path_prefix must be a directory, got %q", pathPrefix)
		}
	} else {
		searchAbs = root
	}

	if exe, ok := lookRipgrep(); ok {
		rgCtx, cancel := context.WithTimeout(ctx, defaultRipgrepTimeout)
		matches, truncated, rgErr := searchRipgrep(rgCtx, exe, root, searchAbs, q, mode, globPat, maxResults, maxPerFile, ctxLines)
		cancel()
		if rgErr == nil {
			return finalizeResult(matches, truncated)
		}
		if len(matches) > 0 {
			return finalizeResult(matches, truncated)
		}
		// No matches or soft failure: try walk fallback
	}

	walkCtx, cancel := context.WithTimeout(ctx, defaultRipgrepTimeout)
	defer cancel()
	matches, truncated, err := searchWalkDir(walkCtx, root, searchAbs, q, mode, globPat, maxResults, maxPerFile, ctxLines)
	if err != nil {
		return nil, err
	}
	return finalizeResult(matches, truncated)
}

func finalizeResult(matches []Match, truncated bool) (map[string]any, error) {
	out := map[string]any{
		"matches": toSliceAny(matches),
	}
	if truncated {
		out["truncated"] = true
	}
	return out, nil
}

func toSliceAny(mm []Match) []any {
	out := make([]any, len(mm))
	for i := range mm {
		out[i] = mm[i]
	}
	return out
}

func intFromArg(m map[string]any, key string, def int) int {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	default:
		return def
	}
}

func lookRipgrep() (exe string, ok bool) {
	candidates := []string{"rg"}
	if runtime.GOOS == "windows" {
		candidates = []string{"rg.exe", "rg"}
	}
	for _, name := range candidates {
		p, err := exec.LookPath(name)
		if err == nil {
			return p, true
		}
	}
	return "", false
}

// ripgrep JSON types (subset).
type rgJSON struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type rgMatchData struct {
	Path       rgTextField `json:"path"`
	Lines      rgTextField `json:"lines"`
	LineNumber int         `json:"line_number"`
	Submatches []struct {
		Start int `json:"start"`
	} `json:"submatches"`
}

type rgTextField struct {
	Text string `json:"text"`
}

func searchRipgrep(ctx context.Context, rgExe, root, searchAbs, query, mode, globPat string, maxResults, maxPerFile, ctxLines int) ([]Match, bool, error) {
	relSearch, err := filepath.Rel(root, searchAbs)
	if err != nil {
		return nil, false, err
	}
	relSearch = filepath.ToSlash(relSearch)
	args := []string{
		"--json",
		"-S",
		"--hidden",
		fmt.Sprintf("--max-filesize=%dM", defaultMaxFileSizeBytes/(1024*1024)),
		"--max-columns", "4096",
	}
	if maxPerFile > 0 {
		args = append(args, "--max-count", fmt.Sprintf("%d", maxPerFile))
	}
	if ctxLines > 0 {
		args = append(args, "-C", fmt.Sprintf("%d", ctxLines))
	}
	// Exclude noisy trees (same spirit as blocklist; rg respects globs)
	for _, seg := range DefaultSkippedPathSegments {
		args = append(args, "--glob", "!**/"+seg+"/**")
	}
	if strings.TrimSpace(globPat) != "" {
		args = append(args, "--glob", globPat)
	}
	if mode == "substring" {
		args = append(args, "-F")
	}
	if strings.HasPrefix(query, "-") {
		args = append(args, "--")
	}
	args = append(args, query)
	if relSearch != "" && relSearch != "." {
		args = append(args, relSearch)
	}

	cmd := exec.CommandContext(ctx, rgExe, args...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, false, err
	}
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		return nil, false, err
	}
	defer func() { _ = stdout.Close() }()

	var matches []Match
	truncated := false
	sc := bufio.NewScanner(stdout)
	// Large lines
	buf := make([]byte, 0, 1024*64)
	sc.Buffer(buf, 1024*1024)

	for sc.Scan() {
		if len(matches) >= maxResults {
			truncated = true
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			break
		}
		line := sc.Bytes()
		var head rgJSON
		if json.Unmarshal(line, &head) != nil || head.Type != "match" {
			continue
		}
		var md rgMatchData
		if json.Unmarshal(head.Data, &md) != nil {
			continue
		}
		p := md.Path.Text
		p = filepath.ToSlash(strings.TrimPrefix(p, "./"))
		col := 1
		if len(md.Submatches) > 0 {
			col = md.Submatches[0].Start + 1
		}
		sn := strings.TrimRight(md.Lines.Text, "\n\r")
		if sn == "" {
			sn = "(empty line)"
		}
		matches = append(matches, Match{
			Path:    p,
			Line:    md.LineNumber,
			Column:  col,
			Snippet: sn,
		})
	}
	waitErr := cmd.Wait()
	if waitErr != nil && len(matches) == 0 {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) && exitErr.ExitCode() == 1 {
			return []Match{}, false, nil
		}
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return matches, truncated, ctx.Err()
		}
		if truncated && errors.As(waitErr, &exitErr) {
			return matches, truncated, sc.Err()
		}
		if stderr.Len() > 0 {
			return matches, truncated, fmt.Errorf("rg: %v: %s", waitErr, strings.TrimSpace(stderr.String()))
		}
		return matches, truncated, waitErr
	}
	return matches, truncated, sc.Err()
}

func searchWalkDir(ctx context.Context, root, searchAbs, query, mode, globPat string, maxResults, maxPerFile, ctxLines int) ([]Match, bool, error) {
	var re *regexp.Regexp
	if mode == "regex" {
		var err error
		re, err = regexp.Compile(query)
		if err != nil {
			return nil, false, fmt.Errorf("invalid regex: %w", err)
		}
	}

	var matches []Match
	truncated := false
	perFile := map[string]int{}

	var rx fileNameGlob
	if globPat != "" {
		var err error
		rx, err = compileNameGlob(globPat)
		if err != nil {
			return nil, false, err
		}
	}

	walkFn := func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if len(matches) >= maxResults {
			truncated = true
			return filepath.SkipAll
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if ShouldSkipPath(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if ShouldSkipPath(rel) {
			return nil
		}
		if globPat != "" && !rx.matchString(filepath.Base(path)) {
			return nil
		}
		if skipBinaryExt(path) {
			return nil
		}
		n := perFile[rel]
		if maxPerFile > 0 && n >= maxPerFile {
			return nil
		}

		fmatches, err := grepFile(ctx, path, query, mode, re, maxResults-len(matches), maxPerFile-n, ctxLines)
		if err != nil {
			return nil
		}
		for _, m := range fmatches {
			m.Path = rel
			perFile[rel]++
			matches = append(matches, m)
			if len(matches) >= maxResults {
				truncated = true
				return filepath.SkipAll
			}
		}
		return nil
	}
	if err := filepath.WalkDir(searchAbs, walkFn); err != nil {
		if errors.Is(err, filepath.SkipAll) {
			return matches, truncated, nil
		}
		return matches, truncated, err
	}
	return matches, truncated, nil
}

func skipBinaryExt(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico", ".exe", ".dll", ".so", ".dylib",
		".zip", ".gz", ".tar", ".pdf", ".woff", ".woff2", ".ttf", ".eot":
		return true
	default:
		return false
	}
}

func grepFile(ctx context.Context, path, query, mode string, re *regexp.Regexp, budget, maxPerFile, ctxLines int) ([]Match, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.Size() > int64(defaultMaxFileSizeBytes) {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil
	}
	defer f.Close()
	head := make([]byte, 4096)
	n, _ := f.Read(head)
	if bytes.IndexByte(head[:n], 0) >= 0 {
		return nil, nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, nil
	}

	var out []Match
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		lineNo++
		if maxPerFile > 0 && len(out) >= maxPerFile {
			break
		}
		if len(out) >= budget {
			break
		}
		t := sc.Text()
		var ok bool
		var col int
		if mode == "substring" {
			idx := strings.Index(t, query)
			ok = idx >= 0
			if ok {
				col = idx + 1
			}
		} else {
			loc := re.FindStringIndex(t)
			ok = loc != nil
			if ok {
				col = loc[0] + 1
			}
		}
		if ok {
			sn := t
			if ctxLines > 0 {
				// keep single-line snippet for walk path; context would need multi-line read
				_ = ctxLines
			}
			out = append(out, Match{
				Line:    lineNo,
				Column:  col,
				Snippet: sn,
			})
		}
	}
	return out, sc.Err()
}

// minimal glob: * and ? only
type fileNameGlob struct {
	pat string
}

func compileNameGlob(pat string) (fileNameGlob, error) {
	if strings.Contains(pat, "**") {
		return fileNameGlob{}, fmt.Errorf("glob ** is not supported in walk mode; omit glob or use ripgrep backend")
	}
	return fileNameGlob{pat: pat}, nil
}

func (g fileNameGlob) matchString(name string) bool {
	ok, _ := filepath.Match(g.pat, name)
	return ok
}

// New returns an ADK function tool.
func New() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "workspace_search",
		Description: desc,
	}, func(_ tool.Context, in args) (map[string]any, error) {
		m := map[string]any{
			"query": in.Query,
		}
		if in.Mode != "" {
			m["mode"] = in.Mode
		}
		if in.PathPrefix != "" {
			m["path_prefix"] = in.PathPrefix
		}
		if in.Glob != "" {
			m["glob"] = in.Glob
		}
		if in.MaxResults != 0 {
			m["max_results"] = in.MaxResults
		}
		if in.MaxMatchesPerFile != 0 {
			m["max_matches_per_file"] = in.MaxMatchesPerFile
		}
		if in.ContextLines != 0 {
			m["context_lines"] = in.ContextLines
		}
		return Run(context.Background(), m)
	})
}

// OpenAIFunctionSpec is used by the legacy OpenAI tool loop.
func OpenAIFunctionSpec() map[string]any {
	return specs.OpenAI("workspace_search", desc, map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Text or pattern to search for",
			},
			"mode": map[string]any{
				"type":        "string",
				"description": `Search mode: "substring" (default) or "regex"`,
				"enum":        []string{"substring", "regex"},
			},
			"path_prefix": map[string]any{
				"type":        "string",
				"description": "Optional directory under workspace root to narrow search",
			},
			"glob": map[string]any{
				"type":        "string",
				"description": "Optional file name glob (e.g. *.go). ** not supported in pure-walk fallback.",
			},
			"max_results": map[string]any{
				"type":        "number",
				"description": "Max matches to return (default 50, max 500)",
			},
			"max_matches_per_file": map[string]any{
				"type":        "number",
				"description": "Cap matches per file (optional)",
			},
			"context_lines": map[string]any{
				"type":        "number",
				"description": "Lines of context (0-5); best-effort with ripgrep backend",
			},
		},
		"required": []string{"query"},
	})
}
