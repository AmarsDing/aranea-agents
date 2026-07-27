package pkginstall

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateManifestRejectsTraversalPaths(t *testing.T) {
	base := &Manifest{Version: 1, Metadata: ManifestMetadata{Name: "pkg"}}
	cases := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{
			name: "skill path",
			mutate: func(m *Manifest) {
				m.Spec.Skills = []SkillSpec{{Path: "../skill.zip"}}
			},
		},
		{
			name: "skill subpath",
			mutate: func(m *Manifest) {
				m.Spec.Skills = []SkillSpec{{URL: "https://example.invalid/repo.git", Subpath: "a/../../b"}}
			},
		},
		{
			name: "graph file",
			mutate: func(m *Manifest) {
				m.Spec.Graphs = []GraphSpec{{File: `..\secret.json`}}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := *base
			tc.mutate(&m)
			if err := ValidateManifest(&m); err == nil {
				t.Fatal("ValidateManifest() error = nil, want traversal rejection")
			}
		})
	}
}

func TestCloneArgsUsesDoubleDashBeforeURL(t *testing.T) {
	args := cloneArgs("--upload-pack=evil", "main", true, "/tmp/pkg")
	joined := strings.Join(args, "\x00")
	if !strings.Contains(joined, "\x00--\x00--upload-pack=evil\x00/tmp/pkg") {
		t.Fatalf("cloneArgs() = %#v, want -- separator before URL", args)
	}
}

func TestFetchFromURLReportsStderrTail(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	missing := filepath.Join(t.TempDir(), "no-such-repo")
	_, _, err := FetchFromURL(missing, "", true)
	if err == nil {
		t.Fatal("FetchFromURL() error = nil, want clone failure")
	}
	// Quiet mode must still surface git's stderr (e.g. "fatal: ..."),
	// not just "exit status 128".
	if !strings.Contains(err.Error(), "fatal") {
		t.Fatalf("FetchFromURL() error = %q, want git stderr tail", err)
	}
	if !strings.Contains(err.Error(), missing) {
		t.Fatalf("FetchFromURL() error = %q, want repo URL %q", err, missing)
	}
}

func TestStderrTailTruncates(t *testing.T) {
	long := strings.Repeat("x", 500)
	if got := stderrTail(long, 200); len(got) != 200 {
		t.Fatalf("stderrTail() len = %d, want 200", len(got))
	}
	if got := stderrTail("  short\n", 200); got != "short" {
		t.Fatalf("stderrTail() = %q, want trimmed %q", got, "short")
	}
	if got := stderrTail("", 200); got != "" {
		t.Fatalf("stderrTail() = %q, want empty", got)
	}
}
