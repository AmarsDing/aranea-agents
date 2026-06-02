package skill

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

type mockLLMGenerator struct {
	response string
	err      error
	delay    time.Duration
}

func (m *mockLLMGenerator) Generate(ctx context.Context, prompt string) (string, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestGenerateSKILLMD_Success(t *testing.T) {
	gen := &mockLLMGenerator{
		response: `NAME: web_search
---
name: web_search
description: Search the web for information
triggers:
  - user asks about current events
steps:
  - Call web_search tool
  - Summarize results
---
# Web Search Skill
Use this skill when the user asks about current events.`,
	}
	creator := NewSkillAutoCreator(gen, nil)
	name, content, err := creator.GenerateSKILLMD(context.Background(), "高频工具调用模式: web_search(15)", []biz.ToolCallRecord{
		{ToolName: "web_search", Arguments: `{"query":"latest news"}`, Result: "results...", Success: true, CalledAt: time.Now()},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "web_search" {
		t.Errorf("name = %q, want %q", name, "web_search")
	}
	if !startsWithYAMLFrontMatter(content) {
		t.Errorf("content should start with YAML front matter, got: %s", truncate(content, 50))
	}
}

func TestGenerateSKILLMD_Timeout(t *testing.T) {
	gen := &mockLLMGenerator{
		response: "ok",
		delay:    60 * time.Second,
	}
	creator := NewSkillAutoCreator(gen, nil)
	_, _, err := creator.GenerateSKILLMD(context.Background(), "pattern", nil)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestGenerateSKILLMD_NoYAMLFrontMatter(t *testing.T) {
	gen := &mockLLMGenerator{
		response: `NAME: bad_skill
This is just plain text without front matter.`,
	}
	creator := NewSkillAutoCreator(gen, nil)
	_, _, err := creator.GenerateSKILLMD(context.Background(), "pattern", nil)
	if err == nil {
		t.Fatal("expected error for missing YAML front matter, got nil")
	}
}

func TestGenerateSKILLMD_EmptyNameUsesHash(t *testing.T) {
	gen := &mockLLMGenerator{
		response: `---
name: ""
description: A skill without a name
---
# Empty Name Skill`,
	}
	creator := NewSkillAutoCreator(gen, nil)
	name, _, err := creator.GenerateSKILLMD(context.Background(), "some pattern", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name == "" {
		t.Error("expected auto-generated name from hash, got empty")
	}
	if len(name) < 5 {
		t.Errorf("auto-generated name seems too short: %q", name)
	}
}

func TestGenerateSKILLMD_GeneratorError(t *testing.T) {
	gen := &mockLLMGenerator{
		err: context.Canceled,
	}
	creator := NewSkillAutoCreator(gen, nil)
	_, _, err := creator.GenerateSKILLMD(context.Background(), "pattern", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func startsWithYAMLFrontMatter(s string) bool {
	return len(s) >= 3 && s[:3] == "---"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
