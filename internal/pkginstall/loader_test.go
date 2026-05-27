package pkginstall

import (
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
