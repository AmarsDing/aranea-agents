package agent

import (
	"trpc.group/trpc-go/trpc-agent-go/prompt"
)

// RenderPromptTemplate renders a prompt template with variable substitution using
// the framework's prompt.Text.Render() with SyntaxMixedBrace (supports both {name}
// and {{name}} placeholders).
//
// This is the framework-aligned rendering approach. Existing manual concatenation
// in prompt.go (BuildSystemPrompt, StaticRuntimeCapabilityCue, etc.) is TECH-DEBT
// to be gradually migrated to this utility.
func RenderPromptTemplate(template string, vars map[string]string) (string, error) {
	t := prompt.Text{
		Template: template,
		Syntax:   prompt.SyntaxMixedBrace,
	}
	env := prompt.RenderEnv{Vars: vars}
	return t.Render(env)
}

// RenderCapabilityCue renders a runtime capability cue template with variable
// substitution. It uses the same SyntaxMixedBrace mode as RenderPromptTemplate
// but is named separately to distinguish capability-cue rendering from general
// prompt rendering, allowing future divergence (e.g. different unknown-behavior
// policy) without breaking callers.
func RenderCapabilityCue(template string, vars map[string]string) (string, error) {
	t := prompt.Text{
		Template: template,
		Syntax:   prompt.SyntaxMixedBrace,
	}
	env := prompt.RenderEnv{Vars: vars}
	return t.Render(env)
}
