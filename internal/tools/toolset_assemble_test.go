package tools

import (
	"context"
	"testing"
)

func TestAssemble_googleSearchWithoutCredentialsSkips(t *testing.T) {
	ctx := context.Background()
	out, err := Assemble(ctx, AssemblyConfig{
		EnabledTools: []string{"google_search"},
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil assembled output")
	}
	for _, ts := range out.ToolSets {
		if ts.Name() == "google_search" {
			t.Fatal("google_search should be skipped without api key and cx")
		}
	}
}

func TestAssemble_geminifetchWithoutModelSkips(t *testing.T) {
	ctx := context.Background()
	out, err := Assemble(ctx, AssemblyConfig{
		EnabledTools: []string{"geminifetch"},
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	for _, tool := range out.Tools {
		if decl := tool.Declaration(); decl != nil && decl.Name == "gemini_web_fetch" {
			t.Fatal("geminifetch should be skipped without model")
		}
	}
}
