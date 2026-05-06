package provider

import (
	"strings"

	"google.golang.org/genai"
)

// TextsFromLLMResponse splits visible assistant text vs reasoning (Thought parts) from one ADK chunk.
func TextsFromLLMResponse(r *LLMResponse) (main string, reasoning string) {
	if r == nil || r.Content == nil {
		return "", ""
	}
	var mainB, reasonB strings.Builder
	for _, p := range r.Content.Parts {
		if p == nil || strings.TrimSpace(p.Text) == "" {
			continue
		}
		if p.Thought {
			reasonB.WriteString(p.Text)
			continue
		}
		mainB.WriteString(p.Text)
	}
	return mainB.String(), reasonB.String()
}

// UsageFromLLMResponse maps genai usage metadata to OpenAI-style int counts.
func UsageFromLLMResponse(r *LLMResponse) (promptTok, completionTok int) {
	if r == nil || r.UsageMetadata == nil {
		return 0, 0
	}
	return int(r.UsageMetadata.PromptTokenCount), int(r.UsageMetadata.CandidatesTokenCount)
}

// DefaultChatConfig returns a minimal GenerateContentConfig (temperature) for chat calls.
func DefaultChatConfig() *genai.GenerateContentConfig {
	t := float32(0.7)
	return &genai.GenerateContentConfig{Temperature: &t}
}
