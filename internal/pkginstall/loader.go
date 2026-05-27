package pkginstall

import (
	"fmt"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"strings"

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

// FetchFromURL clones a git repository (shallow) into a temporary directory.
// Returns the directory path and a cleanup function.
//
// Implementation note: uses os/exec("git clone --depth 1") instead of go-git
// to avoid a large transitive dependency. Requires git in PATH.
func FetchFromURL(repoURL, ref string, quiet bool) (pkgDir string, cleanup func(), err error) {
	tmpDir, err := os.MkdirTemp("", "aranea-pkg-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(tmpDir) }

	args := cloneArgs(repoURL, ref, quiet, tmpDir)

	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if quiet {
		cmd.Stdout = nil
		cmd.Stderr = nil
	}
	if err := cmd.Run(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("git clone %s: %w", repoURL, err)
	}
	return tmpDir, cleanup, nil
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
