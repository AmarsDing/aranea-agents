package security

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

var DefaultProtectedPaths = []string{
	".aws/credentials",
	".aws/config",
	".ssh/*",
	".kube/config",
	".docker/config.json",
	".npmrc",
	".netrc",
	".pypirc",
	".git-credentials",
	".env",
	".bashrc",
	".zshrc",
	".profile",
	".bash_profile",
	".gitconfig",
	"**/.env",
	"**/.env.*",
	"**/credentials.json",
	"**/service-account*.json",
	"**/id_rsa*",
	"**/id_ed25519*",
	"**/id_ecdsa*",
}

var DefaultProtectedTools = map[string]bool{
	"exec_command": true,
	"shell_exec":   true,
	"hostexec":     true,
	"file":         true,
}

type CommandSafetyPolicy struct {
	protectedPaths []string
	protectedTools map[string]bool
}

func NewCommandSafetyPolicy() *CommandSafetyPolicy {
	p := &CommandSafetyPolicy{
		protectedPaths: make([]string, len(DefaultProtectedPaths)),
		protectedTools: make(map[string]bool, len(DefaultProtectedTools)),
	}
	copy(p.protectedPaths, DefaultProtectedPaths)
	for k, v := range DefaultProtectedTools {
		p.protectedTools[k] = v
	}
	return p
}

func NewCommandSafetyPolicyWithConfig(protectedPaths []string, protectedTools map[string]bool) *CommandSafetyPolicy {
	p := &CommandSafetyPolicy{
		protectedPaths: make([]string, len(DefaultProtectedPaths)),
		protectedTools: make(map[string]bool, len(DefaultProtectedTools)),
	}
	copy(p.protectedPaths, DefaultProtectedPaths)
	for k, v := range DefaultProtectedTools {
		p.protectedTools[k] = v
	}
	for _, pp := range protectedPaths {
		p.protectedPaths = append(p.protectedPaths, pp)
	}
	for k, v := range protectedTools {
		p.protectedTools[k] = v
	}
	return p
}

func (p *CommandSafetyPolicy) IsProtectedTool(toolName string) bool {
	return p.protectedTools[toolName]
}

func (p *CommandSafetyPolicy) Evaluate(toolName string, args []byte) *PolicyViolation {
	if !p.IsProtectedTool(toolName) {
		return nil
	}
	paths := extractPathsFromArgs(args)
	for _, path := range paths {
		if violation := p.checkPath(toolName, path); violation != nil {
			return violation
		}
	}
	return nil
}

func (p *CommandSafetyPolicy) checkPath(toolName, path string) *PolicyViolation {
	normalized := normalizePath(path)
	for _, pattern := range p.protectedPaths {
		if matchPath(pattern, normalized) {
			return &PolicyViolation{
				ToolName: toolName,
				Rule:     "sensitive_path_access",
				Path:     normalized,
				Message:  "Access to sensitive path is blocked for security",
			}
		}
	}
	return nil
}

func extractPathsFromArgs(args []byte) []string {
	if len(args) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		return extractPathTokens(string(args))
	}
	var paths []string
	pathKeys := []string{"path", "file_path", "filepath", "filename", "dir", "directory", "cwd", "workdir", "base_dir"}
	for _, key := range pathKeys {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				paths = append(paths, s)
			}
		}
	}
	if v, ok := m["command"]; ok {
		if s, ok := v.(string); ok {
			paths = append(paths, extractPathTokens(s)...)
		}
	}
	return paths
}

func extractPathTokens(s string) []string {
	var paths []string
	fields := strings.Fields(s)
	for _, f := range fields {
		if looksLikePath(f) {
			paths = append(paths, f)
		}
	}
	return paths
}

func looksLikePath(s string) bool {
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || strings.HasPrefix(s, "~") {
		return true
	}
	if len(s) >= 3 && s[1] == ':' && (s[2] == '\\' || s[2] == '/') {
		return true
	}
	if strings.Contains(s, "/") && (strings.Contains(s, ".") || strings.Contains(s, "_")) {
		return true
	}
	for _, ext := range []string{".json", ".yaml", ".yml", ".toml", ".env", ".cfg", ".conf", ".pem", ".key", ".pub"} {
		if strings.HasSuffix(strings.ToLower(s), ext) {
			return true
		}
	}
	return false
}

func normalizePath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if strings.HasPrefix(p, "~/") {
		p = p[1:]
	}
	p = strings.ReplaceAll(filepath.Clean(p), "\\", "/")
	if !strings.HasPrefix(p, "/") && !strings.HasPrefix(p, ".") {
		p = "./" + p
	}
	return p
}

func matchPath(pattern, path string) bool {
	pattern = strings.ReplaceAll(pattern, "\\", "/")
	path = strings.ReplaceAll(path, "\\", "/")

	if !strings.Contains(pattern, "*") {
		patternNorm := normalizePath(pattern)
		pathNorm := normalizePath(path)
		if pathNorm == patternNorm {
			return true
		}
		return strings.HasSuffix(pathNorm, "/"+strings.TrimPrefix(patternNorm, "./"))
	}

	if strings.HasPrefix(pattern, "**/") {
		suffix := pattern[3:]
		base := filepath.Base(path)
		matched, err := filepath.Match(suffix, base)
		if err == nil && matched {
			return true
		}
	}

	segments := strings.Split(strings.Trim(path, "/"), "/")
	patParts := strings.Split(strings.Trim(pattern, "/"), "/")
	for i := 0; i <= len(segments)-len(patParts); i++ {
		allMatch := true
		for j, pp := range patParts {
			matched, err := filepath.Match(pp, segments[i+j])
			if err != nil || !matched {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
	}
	return false
}

type PolicyViolation struct {
	ToolName string
	Rule     string
	Path     string
	Message  string
}

func (v *PolicyViolation) Error() string {
	return v.Message
}
