package skill

import (
	"context"
	"strings"
	"testing"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type mockLLMGenerator struct {
	LLMSkillGenerator
	output string
	err    error
}

func (m *mockLLMGenerator) Generate(_ context.Context, _ string) (string, error) {
	return m.output, m.err
}

func TestSkillAutoCreator_GenerateSKILLMD_Success(t *testing.T) {
	generator := &mockLLMGenerator{
		output: "NAME: web-search\n---\nname: web-search\ndescription: Search the web\n---\n# Web Search Skill\n\nSteps:\n1. Call web_search",
	}
	creator := NewSkillAutoCreator(generator, loggateway.NewNoop())

	name, content, err := creator.GenerateSKILLMD(context.Background(), "web_search(query)", nil)
	if err != nil {
		t.Fatalf("GenerateSKILLMD: %v", err)
	}
	if name != "web-search" {
		t.Errorf("expected name=web-search, got %s", name)
	}
	if !strings.HasPrefix(content, "---") {
		t.Errorf("expected content to start with ---, got %s", content[:20])
	}
}

func TestSkillAutoCreator_GenerateSKILLMD_WithToolHistory(t *testing.T) {
	generator := &mockLLMGenerator{
		output: "NAME: search-and-summarize\n---\nname: search-and-summarize\n---\nbody",
	}
	creator := NewSkillAutoCreator(generator, loggateway.NewNoop())

	history := []biz.ToolCallRecord{
		{ToolName: "web_search", Arguments: "query", Success: true},
		{ToolName: "summarize", Arguments: "text", Success: true},
	}
	name, _, err := creator.GenerateSKILLMD(context.Background(), "web_search + summarize", history)
	if err != nil {
		t.Fatalf("GenerateSKILLMD: %v", err)
	}
	if name != "search-and-summarize" {
		t.Errorf("expected search-and-summarize, got %s", name)
	}
}

func TestSkillAutoCreator_GenerateSKILLMD_GeneratorError(t *testing.T) {
	generator := &mockLLMGenerator{
		err: kerrors.InternalServer("LLM", "model unavailable"),
	}
	creator := NewSkillAutoCreator(generator, loggateway.NewNoop())

	_, _, err := creator.GenerateSKILLMD(context.Background(), "pattern", nil)
	if err == nil {
		t.Fatal("expected error from generator failure")
	}
	if !kerrors.IsInternalServer(err) {
		t.Errorf("expected InternalServer, got %v", err)
	}
}

func TestSkillAutoCreator_GenerateSKILLMD_NoYAMLFrontMatter(t *testing.T) {
	generator := &mockLLMGenerator{
		output: "NAME: bad-skill\nThis is just plain text without front matter",
	}
	creator := NewSkillAutoCreator(generator, loggateway.NewNoop())

	_, _, err := creator.GenerateSKILLMD(context.Background(), "pattern", nil)
	if err == nil {
		t.Fatal("expected error for missing YAML front matter")
	}
	if !kerrors.IsBadRequest(err) {
		t.Errorf("expected BadRequest, got %v", err)
	}
}

func TestSkillAutoCreator_GenerateSKILLMD_AutoNameWhenEmpty(t *testing.T) {
	generator := &mockLLMGenerator{
		output: "---\nname: auto-named\n---\nbody",
	}
	creator := NewSkillAutoCreator(generator, loggateway.NewNoop())

	name, _, err := creator.GenerateSKILLMD(context.Background(), "some pattern", nil)
	if err != nil {
		t.Fatalf("GenerateSKILLMD: %v", err)
	}
	if name == "" {
		t.Error("expected auto-generated name when NAME line is missing")
	}
}

func TestBuildSkillPrompt(t *testing.T) {
	prompt := buildSkillPrompt("web_search(query)", []biz.ToolCallRecord{
		{ToolName: "web_search", Arguments: "test query", Result: "results", Success: true},
	})
	if !strings.Contains(prompt, "web_search(query)") {
		t.Error("prompt should contain pattern description")
	}
	if !strings.Contains(prompt, "web_search(test query)") {
		t.Error("prompt should contain tool call history")
	}
	if !strings.Contains(prompt, "success") {
		t.Error("prompt should contain success status")
	}
}

func TestBuildSkillPrompt_NoHistory(t *testing.T) {
	prompt := buildSkillPrompt("some pattern", nil)
	if !strings.Contains(prompt, "some pattern") {
		t.Error("prompt should contain pattern description")
	}
	if strings.Contains(prompt, "Tool call history") {
		t.Error("prompt should not contain tool call history section when empty")
	}
}

func TestParseSkillOutput_WithNAME(t *testing.T) {
	output := "NAME: my-skill\n---\nname: my-skill\n---\nbody"
	name, content, err := parseSkillOutput(output)
	if err != nil {
		t.Fatalf("parseSkillOutput: %v", err)
	}
	if name != "my-skill" {
		t.Errorf("expected my-skill, got %s", name)
	}
	if !strings.HasPrefix(content, "---") {
		t.Errorf("expected content to start with ---, got %s", content[:20])
	}
}

func TestParseSkillOutput_NoNAME(t *testing.T) {
	output := "---\nname: implicit\n---\nbody"
	name, content, err := parseSkillOutput(output)
	if err != nil {
		t.Fatalf("parseSkillOutput: %v", err)
	}
	if name != "" {
		t.Errorf("expected empty name when no NAME line, got %s", name)
	}
	if !strings.HasPrefix(content, "---") {
		t.Errorf("expected content to start with ---, got %s", content[:20])
	}
}
