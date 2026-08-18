package readlints

import (
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	pyFileLine   = regexp.MustCompile(`File "([^"]+)", line (\d+)`)
	nodeFileLine = regexp.MustCompile(`(?m)^(.+):(\d+)`)
)

func hasLintableNonGo(relPaths []string) bool {
	for _, p := range relPaths {
		switch langOf(p) {
		case "python", "javascript":
			return true
		}
	}
	return false
}

func langOf(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".py":
		return "python"
	case ".js", ".mjs", ".cjs", ".jsx":
		return "javascript"
	default:
		return ""
	}
}

func (l *linter) lintNonGo(ctx context.Context, absPaths, relPaths []string) []diagnostic {
	var out []diagnostic
	for i, abs := range absPaths {
		if len(out) >= maxDiagnostics {
			break
		}
		rel := abs
		if i < len(relPaths) && relPaths[i] != "" {
			rel = relPaths[i]
		}
		switch langOf(abs) {
		case "python":
			out = append(out, l.lintPython(ctx, abs, rel)...)
		case "javascript":
			out = append(out, l.lintJavaScript(ctx, abs, rel)...)
		}
	}
	return out
}

func (l *linter) lintPython(ctx context.Context, abs, rel string) []diagnostic {
	name, extra := pickPython()
	if name == "" {
		name = "python"
	}
	args := append(append([]string{}, extra...), "-m", "py_compile", abs)
	return l.runFileDiag(ctx, name, args, rel, "py_compile", parsePythonDiag)
}

func (l *linter) lintJavaScript(ctx context.Context, abs, rel string) []diagnostic {
	return l.runFileDiag(ctx, "node", []string{"--check", abs}, rel, "node --check", parseNodeDiag)
}

func (l *linter) runFileDiag(ctx context.Context, name string, args []string, rel, source string, parse func(string, string) []diagnostic) []diagnostic {
	ctx, cancel := context.WithTimeout(ctx, vetTimeout)
	defer cancel()
	stdout, stderr, err := l.run(ctx, l.baseDir, name, args)
	combined := strings.TrimSpace(stdout + "\n" + stderr)
	if err == nil {
		return nil
	}
	diags := parse(combined, rel)
	if len(diags) == 0 && combined != "" {
		return []diagnostic{{
			Path:     rel,
			Severity: "error",
			Message:  combined,
			Source:   source,
		}}
	}
	for i := range diags {
		if diags[i].Source == "" {
			diags[i].Source = source
		}
		if diags[i].Path == "" {
			diags[i].Path = rel
		}
	}
	return diags
}

func pickPython() (string, []string) {
	if _, err := exec.LookPath("python"); err == nil {
		return "python", nil
	}
	if _, err := exec.LookPath("python3"); err == nil {
		return "python3", nil
	}
	if _, err := exec.LookPath("py"); err == nil {
		return "py", []string{"-3"}
	}
	return "", nil
}

func parsePythonDiag(text, rel string) []diagnostic {
	m := pyFileLine.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	msg := text
	if i := strings.LastIndex(text, "SyntaxError:"); i >= 0 {
		msg = strings.TrimSpace(text[i:])
	}
	return []diagnostic{{
		Path:     rel,
		Line:     atoi(m[2]),
		Severity: "error",
		Message:  msg,
		Source:   "py_compile",
	}}
}

func parseNodeDiag(text, rel string) []diagnostic {
	line := 0
	for _, row := range strings.Split(text, "\n") {
		m := nodeFileLine.FindStringSubmatch(strings.TrimSpace(row))
		if m != nil {
			line = atoi(m[2])
			break
		}
	}
	msg := strings.TrimSpace(text)
	if i := strings.Index(msg, "SyntaxError:"); i >= 0 {
		msg = strings.TrimSpace(msg[i:])
	}
	if msg == "" {
		return nil
	}
	return []diagnostic{{
		Path:     rel,
		Line:     line,
		Severity: "error",
		Message:  msg,
		Source:   "node --check",
	}}
}
