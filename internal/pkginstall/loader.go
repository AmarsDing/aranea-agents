package pkginstall

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LoadManifestFromDir reads aranea-package.yaml from the given directory.
func LoadManifestFromDir(dir string) (*Manifest, error) {
	path := filepath.Join(dir, "aranea-package.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		// Try .yml extension as fallback.
		path = filepath.Join(dir, "aranea-package.yml")
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("aranea-package.yaml not found in %s", dir)
		}
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Version == 0 {
		m.Version = 1
	}
	return &m, nil
}

// cloneMaxAttempts bounds how many times a failed git clone is retried.
// GitHub connectivity from CN networks flaps frequently (TLS resets /
// connect timeouts), so a single attempt is not reliable in practice.
const cloneMaxAttempts = 3

// cloneBackoff returns the wait before the next clone attempt (attempt is
// 1-based: backoff(1) is the wait after the first failure). Swappable in tests.
var cloneBackoff = func(attempt int) time.Duration {
	return time.Duration(attempt) * 2 * time.Second
}

// FetchFromURL clones a git repository (shallow) into a temporary directory.
// Returns the directory path and a cleanup function.
//
// Implementation note: uses os/exec("git clone --depth 1") instead of go-git
// to avoid a large transitive dependency. Requires git in PATH.
func FetchFromURL(repoURL, ref string, quiet bool) (pkgDir string, cleanup func(), err error) {
	var lastErr error
	for attempt := 1; attempt <= cloneMaxAttempts; attempt++ {
		pkgDir, cleanup, err = fetchFromURLOnce(repoURL, ref, quiet)
		if err == nil {
			return pkgDir, cleanup, nil
		}
		lastErr = err
		if attempt < cloneMaxAttempts {
			time.Sleep(cloneBackoff(attempt))
		}
	}
	return "", nil, lastErr
}

// fetchFromURLOnce performs a single clone attempt.
func fetchFromURLOnce(repoURL, ref string, quiet bool) (pkgDir string, cleanup func(), err error) {
	tmpDir, err := os.MkdirTemp("", "aranea-pkg-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(tmpDir) }

	args := cloneArgs(repoURL, ref, quiet, tmpDir)

	cmd := exec.Command("git", args...)
	cmd.Env = cloneEnv()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	var stderrBuf bytes.Buffer
	if quiet {
		// Keep stderr even in quiet mode so clone failures are diagnosable
		// (otherwise all the caller sees is "exit status 128").
		cmd.Stdout = nil
		cmd.Stderr = &stderrBuf
	}
	if err := cmd.Run(); err != nil {
		cleanup()
		if tail := stderrTail(stderrBuf.String(), 200); tail != "" {
			return "", nil, fmt.Errorf("git clone %s: %s (%v)", repoURL, tail, err)
		}
		return "", nil, fmt.Errorf("git clone %s: %w", repoURL, err)
	}
	return tmpDir, cleanup, nil
}

// cloneEnv builds the environment for the git subprocess. When
// ARANEA_GIT_PROXY is set (e.g. socks5://127.0.0.1:1080), it overrides any
// pre-existing HTTP(S)_PROXY/ALL_PROXY so git clones route through the
// configured proxy; otherwise the process environment passes through as-is.
func cloneEnv() []string {
	env := os.Environ()
	proxy := strings.TrimSpace(os.Getenv("ARANEA_GIT_PROXY"))
	if proxy == "" {
		return env
	}
	out := make([]string, 0, len(env)+3)
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if strings.EqualFold(k, "HTTP_PROXY") || strings.EqualFold(k, "HTTPS_PROXY") || strings.EqualFold(k, "ALL_PROXY") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "HTTP_PROXY="+proxy, "HTTPS_PROXY="+proxy, "ALL_PROXY="+proxy)
}

// stderrTail returns the trailing slice of s (at most max chars), trimmed.
func stderrTail(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) > max {
		s = s[len(s)-max:]
	}
	return s
}

func cloneArgs(repoURL, ref string, quiet bool, tmpDir string) []string {
	args := []string{"clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	if quiet {
		args = append(args, "--quiet")
	}
	return append(args, "--", repoURL, tmpDir)
}

// ValidateManifest performs basic validation on the manifest.
func ValidateManifest(m *Manifest) error {
	if m == nil {
		return fmt.Errorf("manifest is nil")
	}
	if m.Version != 1 {
		return fmt.Errorf("unsupported manifest version %d (expected 1)", m.Version)
	}
	if m.Metadata.Name == "" {
		return fmt.Errorf("manifest.metadata.name is required")
	}
	for i, spec := range m.Spec.Skills {
		if err := validateManifestPath(spec.Path); err != nil {
			return fmt.Errorf("manifest.spec.skills[%d].path: %w", i, err)
		}
		if err := validateManifestPath(spec.Subpath); err != nil {
			return fmt.Errorf("manifest.spec.skills[%d].subpath: %w", i, err)
		}
	}
	for i, spec := range m.Spec.Graphs {
		if err := validateManifestPath(spec.File); err != nil {
			return fmt.Errorf("manifest.spec.graphs[%d].file: %w", i, err)
		}
	}
	return nil
}

func validateManifestPath(p string) error {
	p = strings.TrimSpace(p)
	if p == "" {
		return nil
	}
	if filepath.IsAbs(p) || pathpkg.IsAbs(strings.ReplaceAll(p, "\\", "/")) || filepath.VolumeName(p) != "" {
		return fmt.Errorf("absolute paths are not allowed")
	}
	for _, part := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return fmt.Errorf("path traversal is not allowed")
		}
	}
	clean := pathpkg.Clean(strings.ReplaceAll(p, "\\", "/"))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("path traversal is not allowed")
	}
	return nil
}
