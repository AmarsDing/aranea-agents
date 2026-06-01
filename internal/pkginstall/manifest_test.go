package pkginstall

import (
	"testing"
)

func TestValidateManifest_Valid(t *testing.T) {
	m := &Manifest{
		Version:  1,
		Metadata: ManifestMetadata{Name: "test-pkg"},
	}
	if err := ValidateManifest(m); err != nil {
		t.Fatal(err)
	}
}

func TestValidateManifest_InvalidVersion(t *testing.T) {
	m := &Manifest{Version: 0, Metadata: ManifestMetadata{Name: "pkg"}}
	if err := ValidateManifest(m); err == nil {
		t.Fatal("expected error for version 0")
	}
}

func TestValidateManifest_EmptyName(t *testing.T) {
	m := &Manifest{Version: 1, Metadata: ManifestMetadata{Name: ""}}
	if err := ValidateManifest(m); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestValidateManifest_SkillURL(t *testing.T) {
	m := &Manifest{
		Version:  1,
		Metadata: ManifestMetadata{Name: "pkg"},
		Spec: ManifestSpec{
			Skills: []SkillSpec{{URL: "https://example.com/repo.git"}},
		},
	}
	if err := ValidateManifest(m); err != nil {
		t.Fatal(err)
	}
}

func TestValidateManifest_SkillPathTraversal(t *testing.T) {
	m := &Manifest{
		Version:  1,
		Metadata: ManifestMetadata{Name: "pkg"},
		Spec: ManifestSpec{
			Skills: []SkillSpec{{Path: "../../etc/passwd"}},
		},
	}
	if err := ValidateManifest(m); err == nil {
		t.Fatal("expected path traversal error")
	}
}

func TestValidateManifest_SkillSubpathTraversal(t *testing.T) {
	m := &Manifest{
		Version:  1,
		Metadata: ManifestMetadata{Name: "pkg"},
		Spec: ManifestSpec{
			Skills: []SkillSpec{{URL: "https://example.com/repo.git", Subpath: "a/../../b"}},
		},
	}
	if err := ValidateManifest(m); err == nil {
		t.Fatal("expected subpath traversal error")
	}
}

func TestValidateManifest_GraphFileTraversal(t *testing.T) {
	m := &Manifest{
		Version:  1,
		Metadata: ManifestMetadata{Name: "pkg"},
		Spec: ManifestSpec{
			Graphs: []GraphSpec{{File: "../secret.json"}},
		},
	}
	if err := ValidateManifest(m); err == nil {
		t.Fatal("expected graph file traversal error")
	}
}

func TestValidateManifest_GraphValidFile(t *testing.T) {
	m := &Manifest{
		Version:  1,
		Metadata: ManifestMetadata{Name: "pkg"},
		Spec: ManifestSpec{
			Graphs: []GraphSpec{{File: "graphs/my_graph.json", Name: "test"}},
		},
	}
	if err := ValidateManifest(m); err != nil {
		t.Fatal(err)
	}
}

func TestValidateManifest_MCPServerSpec(t *testing.T) {
	m := &Manifest{
		Version:  1,
		Metadata: ManifestMetadata{Name: "pkg"},
		Spec: ManifestSpec{
			MCPServers: []MCPServerSpec{{Name: "my-server", Key: "my-server", URL: "https://mcp.example.com", Type: "sse", Enabled: true}},
		},
	}
	if err := ValidateManifest(m); err != nil {
		t.Fatal(err)
	}
}

func TestCloneArgs_Branch(t *testing.T) {
	args := cloneArgs("", "main", false, "/tmp/pkg")
	found := false
	for _, a := range args {
		if a == "main" {
			found = true
		}
	}
	if !found {
		t.Fatalf("branch not in args: %v", args)
	}
}

func TestCloneArgs_Quiet(t *testing.T) {
	args := cloneArgs("", "main", true, "/tmp/pkg")
	found := false
	for _, a := range args {
		if a == "--quiet" {
			found = true
		}
	}
	if !found {
		t.Fatalf("quiet flag not in args: %v", args)
	}
}
