package biz

import (
	"context"
	"regexp"
	"strings"
)

const (
	MemoryLayerL3 = "L3"
)

// ConsolidateMessage is one turn message considered for memory extraction.
type ConsolidateMessage struct {
	Role           string
	Content        string
	MessageID      string
}

// ConsolidateInput is the async consolidator payload after a turn completes.
type ConsolidateInput struct {
	SessionID string
	AgentID   string
	UserID    string
	AppName   string
	Messages  []ConsolidateMessage
}

// MemoryProposal is one derived memory write candidate from a consolidator.
type MemoryProposal struct {
	Layer           string
	Statement       string
	Topics          []string
	SourceMessageID string
}

// MemoryConsolidator extracts structured memory proposals from recent messages.
type MemoryConsolidator interface {
	Extract(ctx context.Context, in ConsolidateInput) ([]MemoryProposal, error)
}

var defaultHeuristicPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:my name is|I(?:'m| am) called)\s+([A-Z][a-z]+(?: [A-Z][a-z]+)?)`),
	regexp.MustCompile(`(?i)I(?:'m| am)\s+(?:a |an )?([a-z]+(?:\s+[a-z]+)?)\s*(?:\.|,|$)`),
	regexp.MustCompile(`(?i)I\s+(?:prefer|like|love|hate|dislike)\s+([^.!?\n]+)`),
	regexp.MustCompile(`(?i)(?:please|always|never)\s+(?:call me|refer to me as)\s+([^.!?\n]+)`),
}

// HeuristicConsolidator applies regex patterns without an LLM call.
type HeuristicConsolidator struct {
	patterns []*regexp.Regexp
}

func NewHeuristicConsolidator() *HeuristicConsolidator {
	return &HeuristicConsolidator{patterns: defaultHeuristicPatterns}
}

func (c *HeuristicConsolidator) Extract(_ context.Context, in ConsolidateInput) ([]MemoryProposal, error) {
	if c == nil {
		return nil, nil
	}
	seen := make(map[string]struct{})
	var out []MemoryProposal
	for _, msg := range in.Messages {
		if strings.TrimSpace(msg.Role) != "user" {
			continue
		}
		text := strings.TrimSpace(msg.Content)
		if text == "" {
			continue
		}
		for _, pat := range c.patterns {
		m := pat.FindStringSubmatch(text)
		if len(m) <= 1 {
			continue
		}
		stmt := strings.TrimSpace(m[1])
		if stmt == "" {
			continue
		}
		key := strings.ToLower(stmt)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, MemoryProposal{
			Layer:           MemoryLayerL3,
			Statement:       stmt,
			SourceMessageID: strings.TrimSpace(msg.MessageID),
		})
		}
	}
	return out, nil
}

// ChainConsolidator tries primary then fallback (typically LLM → heuristic).
type ChainConsolidator struct {
	Primary  MemoryConsolidator
	Fallback MemoryConsolidator
}

func NewChainConsolidator(primary, fallback MemoryConsolidator) *ChainConsolidator {
	return &ChainConsolidator{Primary: primary, Fallback: fallback}
}

func (c *ChainConsolidator) Extract(ctx context.Context, in ConsolidateInput) ([]MemoryProposal, error) {
	return c.extract(ctx, in, nil)
}

func (c *ChainConsolidator) extract(ctx context.Context, in ConsolidateInput, hook func(primaryErr error)) ([]MemoryProposal, error) {
	if c == nil {
		return nil, nil
	}
	var primaryErr error
	if c.Primary != nil {
		props, err := c.Primary.Extract(ctx, in)
		if err == nil && len(props) > 0 {
			return props, nil
		}
		primaryErr = err
	}
	if c.Fallback != nil {
		props, err := c.Fallback.Extract(ctx, in)
		if primaryErr != nil && err == nil && len(props) > 0 && hook != nil {
			hook(primaryErr)
		}
		return props, err
	}
	return nil, primaryErr
}

// ExtractWithFallbackHook runs consolidation and invokes hook when primary fails but fallback succeeds.
func ExtractWithFallbackHook(c MemoryConsolidator, ctx context.Context, in ConsolidateInput, hook func(primaryErr error)) ([]MemoryProposal, error) {
	if chain, ok := c.(*ChainConsolidator); ok {
		return chain.extract(ctx, in, hook)
	}
	return c.Extract(ctx, in)
}

// FeedbackConsolidator builds one L3 proposal from user thumbs up/down feedback.
type FeedbackConsolidator struct{}

func NewFeedbackConsolidator() *FeedbackConsolidator { return &FeedbackConsolidator{} }

func (c *FeedbackConsolidator) Extract(_ context.Context, in ConsolidateInput) ([]MemoryProposal, error) {
	if c == nil {
		return nil, nil
	}
	for _, msg := range in.Messages {
		if strings.TrimSpace(msg.Role) != "feedback" {
			continue
		}
		stmt := strings.TrimSpace(msg.Content)
		if stmt == "" {
			return nil, nil
		}
		return []MemoryProposal{{
			Layer:           MemoryLayerL3,
			Statement:       stmt,
			Topics:          []string{"feedback", "preference"},
			SourceMessageID: strings.TrimSpace(msg.MessageID),
		}}, nil
	}
	return nil, nil
}

func BuildFeedbackStatement(rating, comment, assistantPreview string) string {
	switch strings.TrimSpace(rating) {
	case "negative":
		if comment != "" {
			return "User disliked an assistant response and noted: " + comment
		}
		if assistantPreview != "" {
			return "User disliked an assistant response about: " + assistantPreview
		}
		return "User marked an assistant response as unsatisfactory"
	case "positive":
		if comment != "" {
			return "User approved an assistant response and noted: " + comment
		}
	}
	return ""
}

// ResolveProposalMessageID maps an extracted statement back to the originating user message when possible.
func ResolveProposalMessageID(statement string, messages []ConsolidateMessage) string {
	stmt := strings.ToLower(strings.TrimSpace(statement))
	if stmt == "" {
		return lastUserConsolidateMessageID(messages)
	}
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if strings.TrimSpace(msg.Role) != "user" {
			continue
		}
		content := strings.ToLower(strings.TrimSpace(msg.Content))
		if content == "" {
			continue
		}
		if strings.Contains(content, stmt) || strings.Contains(stmt, content) {
			return strings.TrimSpace(msg.MessageID)
		}
	}
	return lastUserConsolidateMessageID(messages)
}

func lastUserConsolidateMessageID(messages []ConsolidateMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.TrimSpace(messages[i].Role) == "user" {
			if id := strings.TrimSpace(messages[i].MessageID); id != "" {
				return id
			}
		}
	}
	return ""
}
