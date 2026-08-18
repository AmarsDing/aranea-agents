package readlints

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/internal/tools/editstamp"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestReadLintsLiveGoVet(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "go.mod"), []byte("module lintlive.test\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(base, "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package pkg\n\nimport \"fmt\"\n\nfunc F() { fmt.Printf(\"%s\", 1) }\n"
	if err := os.WriteFile(filepath.Join(pkg, "bad.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := NewTool(base, loggateway.NewNoop())
	ct := tool.(trpctool.CallableTool)
	out, err := ct.Call(context.Background(), []byte(`{"path":"pkg/bad.go"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), "fmt.Printf") && !strings.Contains(string(raw), "wrong type") && !strings.Contains(string(raw), "format") {
		t.Fatalf("expected go vet diagnostic, got %s", raw)
	}
}

func TestReadLintsLiveEmptyUsesStamp(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "go.mod"), []byte("module lintlive.test\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(base, "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package pkg\n\nimport \"fmt\"\n\nfunc F() { fmt.Printf(\"%s\", 1) }\n"
	if err := os.WriteFile(filepath.Join(pkg, "bad.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	editstamp.Record(base, "pkg/bad.go")
	tool := NewTool(base, loggateway.NewNoop())
	ct := tool.(trpctool.CallableTool)
	out, err := ct.Call(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), "fmt.Printf") && !strings.Contains(string(raw), "wrong type") && !strings.Contains(string(raw), "format") {
		t.Fatalf("empty path should lint stamped go file, got %s", raw)
	}
}

func TestReadLintsLivePython(t *testing.T) {
	if name, _ := pickPython(); name == "" {
		t.Skip("python not installed")
	}
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "bad.py"), []byte("x =\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := NewTool(base, loggateway.NewNoop())
	ct := tool.(trpctool.CallableTool)
	out, err := ct.Call(context.Background(), []byte(`{"path":"bad.py"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), "SyntaxError") && !strings.Contains(string(raw), "invalid syntax") {
		t.Fatalf("expected python diagnostic, got %s", raw)
	}
}
