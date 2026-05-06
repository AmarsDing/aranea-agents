package provider

import "google.golang.org/adk/model"

// ADK-aligned type aliases — implementations live in provider/openai, provider/deepseek, provider/gemini.
type (
	LLM         = model.LLM
	LLMRequest  = model.LLMRequest
	LLMResponse = model.LLMResponse
)
