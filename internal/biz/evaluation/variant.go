package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type runOverrideKey struct{}

// RunOverride is a per-turn experiment cell (model and/or prompt overlay).
type RunOverride struct {
	Model  string
	Prompt string
	Tools  string
}

// WithRunOverride attaches a matrix cell override to ctx for the eval turn.
func WithRunOverride(ctx context.Context, ov RunOverride) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	ov.Model = strings.TrimSpace(ov.Model)
	ov.Prompt = strings.TrimSpace(ov.Prompt)
	ov.Tools = strings.TrimSpace(ov.Tools)
	if ov.Model == "" && ov.Prompt == "" && ov.Tools == "" {
		return ctx
	}
	return context.WithValue(ctx, runOverrideKey{}, ov)
}

// RunOverrideFrom returns the experiment override stored on ctx.
func RunOverrideFrom(ctx context.Context) (RunOverride, bool) {
	if ctx == nil {
		return RunOverride{}, false
	}
	ov, ok := ctx.Value(runOverrideKey{}).(RunOverride)
	return ov, ok
}

// OverlayPrompt prefixes a user case with an experiment instruction.
func OverlayPrompt(prompt, userInput string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return userInput
	}
	return "[Evaluation experiment instruction]\n" + prompt + "\n\n[User request]\n" + userInput
}

// DefaultVariantLabel builds a stable cell label from agent / model / prompt / tools.
func DefaultVariantLabel(agentID, model, prompt, tools string) string {
	parts := []string{strings.TrimSpace(agentID)}
	if m := strings.TrimSpace(model); m != "" {
		parts = append(parts, m)
	}
	if p := strings.TrimSpace(prompt); p != "" {
		sum := sha256.Sum256([]byte(p))
		parts = append(parts, "p"+hex.EncodeToString(sum[:])[:6])
	}
	if t := strings.TrimSpace(tools); t != "" {
		parts = append(parts, "t"+t)
	}
	return strings.Join(parts, "/")
}

// ParseToolsOverride interprets "none" or a comma-separated allowlist.
func ParseToolsOverride(raw string) (none bool, allow []string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false, nil
	}
	if strings.EqualFold(raw, "none") {
		return true, nil
	}
	seen := map[string]struct{}{}
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		allow = append(allow, p)
	}
	return false, allow
}
