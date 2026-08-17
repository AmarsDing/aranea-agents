package readlints

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"aranea-agents/internal/tools/document"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

const (
	toolName       = "read_lints"
	maxPaths       = 32
	maxDiagnostics = 80
	vetTimeout     = 45 * time.Second
)

// Runner runs a host command. Tests inject a fake.
type Runner func(ctx context.Context, dir, name string, args []string) (stdout, stderr string, err error)

type input struct {
	Path  string   `json:"path,omitempty" jsonschema:"description=Workspace-relative file or directory to lint"`
	Paths []string `json:"paths,omitempty" jsonschema:"description=Optional list of files or directories; defaults to Path"`
}

type diagnostic struct {
	Path     string `json:"path"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Source   string `json:"source"`
}

type output struct {
	Diagnostics []diagnostic `json:"diagnostics"`
	Packages    []string     `json:"packages,omitempty"`
	Message     string       `json:"message"`
}

var diagLine = regexp.MustCompile(`^(.+?):(\d+):(\d+):\s*(.+)$`)

// NewTool returns read_lints for the given workspace root.
func NewTool(baseDir string, lg loggateway.Logger) trpctool.Tool {
	return newTool(baseDir, lg, execRunner)
}

func newTool(baseDir string, lg loggateway.Logger, run Runner) trpctool.Tool {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	if run == nil {
		run = execRunner
	}
	t := &linter{baseDir: baseDir, lg: lg, run: run}
	return trpcfunction.NewFunctionTool(
		t.execute,
		trpcfunction.WithName(toolName),
		trpcfunction.WithDescription("Read compiler/linter diagnostics for workspace files after edits. Pass paths of files you changed. Currently runs go vet on Go packages. Use this instead of guessing whether a change compiles."),
	)
}

type linter struct {
	baseDir string
	lg      loggateway.Logger
	run     Runner
}

func (l *linter) execute(ctx context.Context, in input) (output, error) {
	paths := make([]string, 0, len(in.Paths)+1)
	if strings.TrimSpace(in.Path) != "" {
		paths = append(paths, in.Path)
	}
	paths = append(paths, in.Paths...)
	if len(paths) == 0 {
		return output{Diagnostics: []diagnostic{}, Message: "no paths provided; pass path or paths of files you edited"}, nil
	}
	if len(paths) > maxPaths {
		paths = paths[:maxPaths]
	}
	absPaths := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		candidate := p
		if l.baseDir != "" && !filepath.IsAbs(p) {
			candidate = filepath.Join(l.baseDir, p)
		}
		abs, err := document.ValidatePath(candidate, l.baseDir)
		if err != nil {
			return output{}, err
		}
		absPaths = append(absPaths, abs)
	}
	pkgs := goPackages(l.baseDir, absPaths)
	if len(pkgs) == 0 {
		return output{Diagnostics: []diagnostic{}, Message: "no Go packages in the given paths; other languages are not linted yet"}, nil
	}
	var diags []diagnostic
	for _, pkg := range pkgs {
		more, err := l.vetPackage(ctx, pkg)
		if err != nil {
			l.lg.Debug("go vet failed", loggateway.StepID("tool.read_lints.vet"), loggateway.Err(err), loggateway.Str("package", pkg))
			diags = append(diags, diagnostic{
				Path:     pkg,
				Severity: "error",
				Message:  err.Error(),
				Source:   "go vet",
			})
			continue
		}
		diags = append(diags, more...)
		if len(diags) >= maxDiagnostics {
			diags = diags[:maxDiagnostics]
			break
		}
	}
	msg := "no diagnostics"
	if len(diags) > 0 {
		msg = "found issues; fix these before continuing"
	}
	return output{Diagnostics: diags, Packages: pkgs, Message: msg}, nil
}

func (l *linter) vetPackage(ctx context.Context, pkg string) ([]diagnostic, error) {
	ctx, cancel := context.WithTimeout(ctx, vetTimeout)
	defer cancel()
	stdout, stderr, err := l.run(ctx, l.baseDir, "go", []string{"vet", pkg})
	combined := strings.TrimSpace(stdout + "\n" + stderr)
	if err != nil && combined == "" {
		return nil, err
	}
	diags := parseVetOutput(combined, l.baseDir)
	if err != nil && len(diags) == 0 {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return []diagnostic{{
				Path:     pkg,
				Severity: "error",
				Message:  combined,
				Source:   "go vet",
			}}, nil
		}
		return nil, err
	}
	return diags, nil
}

func parseVetOutput(text, baseDir string) []diagnostic {
	var out []diagnostic
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := diagLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		path := m[1]
		if baseDir != "" {
			if rel, err := filepath.Rel(baseDir, path); err == nil && !strings.HasPrefix(rel, "..") {
				path = filepath.ToSlash(rel)
			} else {
				path = filepath.ToSlash(path)
			}
		}
		out = append(out, diagnostic{
			Path:     path,
			Line:     atoi(m[2]),
			Column:   atoi(m[3]),
			Severity: "error",
			Message:  m[4],
			Source:   "go vet",
		})
		if len(out) >= maxDiagnostics {
			break
		}
	}
	return out
}

func goPackages(baseDir string, absPaths []string) []string {
	seen := map[string]struct{}{}
	var pkgs []string
	for _, abs := range absPaths {
		st, err := os.Stat(abs)
		dir := abs
		if err == nil && !st.IsDir() {
			if !strings.EqualFold(filepath.Ext(abs), ".go") {
				continue
			}
			dir = filepath.Dir(abs)
		} else if err != nil {
			if !strings.EqualFold(filepath.Ext(abs), ".go") {
				continue
			}
			dir = filepath.Dir(abs)
		}
		rel := "."
		if baseDir != "" {
			r, err := filepath.Rel(baseDir, dir)
			if err != nil || strings.HasPrefix(r, "..") {
				continue
			}
			rel = filepath.ToSlash(r)
			if rel == "." || rel == "" {
				rel = "."
			} else {
				rel = "./" + rel
			}
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		pkgs = append(pkgs, rel)
	}
	sort.Strings(pkgs)
	return pkgs
}

func execRunner(ctx context.Context, dir, name string, args []string) (stdout, stderr string, err error) {
	if _, lookErr := exec.LookPath(name); lookErr != nil {
		return "", "", lookErr
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}
