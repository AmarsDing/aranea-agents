package readlints

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestParseVetOutput(t *testing.T) {
	text := `# aranea-agents/internal/foo
internal/foo/bar.go:10:2: unreachable code
# skip
`
	dir := filepath.Join("internal", "foo")
	diags := parseVetOutput(text, "")
	if len(diags) != 1 || diags[0].Line != 10 || !strings.Contains(diags[0].Message, "unreachable") {
		t.Fatalf("%+v", diags)
	}
	_ = dir
}

func TestGoPackagesFromFiles(t *testing.T) {
	base := t.TempDir()
	pkg := filepath.Join(base, "pkg", "x")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(pkg, "a.go")
	if err := os.WriteFile(f, []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := goPackages(base, []string{f, f})
	if len(got) != 1 || got[0] != "./pkg/x" {
		t.Fatalf("%v", got)
	}
}

func TestReadLintsEmptyPaths(t *testing.T) {
	tool := newTool(t.TempDir(), loggateway.NewNoop(), func(context.Context, string, string, []string) (string, string, error) {
		t.Fatal("must not run")
		return "", "", nil
	})
	ct := tool.(trpctool.CallableTool)
	out, err := ct.Call(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), "no paths") {
		t.Fatalf("%s", raw)
	}
}

func TestReadLintsUsesVetRunner(t *testing.T) {
	base := t.TempDir()
	pkg := filepath.Join(base, "pkg")
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(pkg, "a.go")
	if err := os.WriteFile(f, []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tool := newTool(base, loggateway.NewNoop(), func(_ context.Context, dir, name string, args []string) (string, string, error) {
		if name != "go" || args[0] != "vet" {
			t.Fatalf("%s %v", name, args)
		}
		return "", "pkg/a.go:2:1: fmt.Printf format %s has arg of wrong type", nil
	})
	ct := tool.(trpctool.CallableTool)
	out, err := ct.Call(context.Background(), []byte(`{"path":"pkg/a.go"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(out)
	if !strings.Contains(string(raw), "fmt.Printf") {
		t.Fatalf("%s", raw)
	}
}
