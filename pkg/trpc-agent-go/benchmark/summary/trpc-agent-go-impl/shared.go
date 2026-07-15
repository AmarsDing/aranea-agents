//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package main

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	sessionsummary "trpc.group/trpc-go/trpc-agent-go/session/summary"
)

// TokenUsage stores detailed token usage for a single LLM call.
type TokenUsage struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
}

// ToolCallStats stores how many times each tool was invoked during one run.
type ToolCallStats struct {
	Counts map[string]int `json:"counts,omitempty"`
}

type ToolTrace struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Response  string `json:"response,omitempty"`
}

func summaryOptions(cfg *appConfig) []sessionsummary.Option {
	opts := []sessionsummary.Option{
		sessionsummary.WithChecksAny(
			sessionsummary.CheckEventThreshold(cfg.Events),
		),
	}
	if cfg.DetailedPrompt {
		opts = append(opts, sessionsummary.WithDetailedContinuityPrompt())
	}
	return opts
}

func logDetailedPromptConfig(enabled bool) {
	if enabled {
		log.Printf("Detailed Continuity Prompt: true")
		log.Printf("  - sessionsummary.WithDetailedContinuityPrompt() enabled:")
		log.Printf("    * nine-section structured summary prompt")
		log.Printf("    * <analysis> scratchpad stripped from persisted summary")
		log.Printf("    * verbatim user-message appendix appended to summary")
		return
	}
	log.Printf("Detailed Continuity Prompt: false (using framework default summarizer prompt)")
}

func (s *ToolCallStats) increment(toolName string) {
	if s == nil {
		return
	}
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return
	}
	if s.Counts == nil {
		s.Counts = make(map[string]int)
	}
	s.Counts[toolName]++
}

func (s *ToolCallStats) Count(toolName string) int {
	if s == nil || s.Counts == nil {
		return 0
	}
	return s.Counts[toolName]
}

func consumeEvents(evtCh <-chan *event.Event) (string, *TokenUsage) {
	response, usage, _ := consumeEventsWithToolStats(evtCh)
	return response, usage
}

func consumeEventsWithToolStats(evtCh <-chan *event.Event) (string, *TokenUsage, *ToolCallStats) {
	var response strings.Builder
	usage := &TokenUsage{}
	toolStats := &ToolCallStats{}
	seenToolCalls := make(map[string]struct{})

	for evt := range evtCh {
		if evt.Error != nil {
			continue
		}
		if evt.Response == nil {
			continue
		}
		if evt.Response.Usage != nil {
			usage.PromptTokens = evt.Response.Usage.PromptTokens
			usage.CompletionTokens = evt.Response.Usage.CompletionTokens
			usage.TotalTokens = evt.Response.Usage.TotalTokens
		}
		recordToolCalls(evt.Response, toolStats, seenToolCalls)
		if len(evt.Response.Choices) == 0 {
			continue
		}
		choice := evt.Response.Choices[0]
		if choice.Message.Content != "" {
			response.WriteString(choice.Message.Content)
		}
		if choice.Delta.Content != "" {
			response.WriteString(choice.Delta.Content)
		}
	}
	return response.String(), usage, toolStats
}

func consumeFinalAssistantAnswerWithToolStats(
	evtCh <-chan *event.Event,
) (string, *TokenUsage, *ToolCallStats) {
	answer, usage, toolStats, _ := consumeFinalAssistantAnswerWithDetails(evtCh)
	return answer, usage, toolStats
}

func consumeFinalAssistantAnswerWithDetails(
	evtCh <-chan *event.Event,
) (string, *TokenUsage, *ToolCallStats, []ToolTrace) {
	usage := &TokenUsage{}
	toolStats := &ToolCallStats{}
	seenToolCalls := make(map[string]struct{})
	traceRecorder := newToolTraceRecorder()

	var finalAnswer string
	for evt := range evtCh {
		if evt.Error != nil || evt.Response == nil {
			continue
		}
		if evt.Response.Usage != nil {
			usage.PromptTokens = evt.Response.Usage.PromptTokens
			usage.CompletionTokens = evt.Response.Usage.CompletionTokens
			usage.TotalTokens = evt.Response.Usage.TotalTokens
		}
		recordToolCalls(evt.Response, toolStats, seenToolCalls)
		traceRecorder.RecordResponse(evt.Response)
		finalAnswer = pickLatestFinalAssistantContent(finalAnswer, evt.Response)
	}
	return finalAnswer, usage, toolStats, traceRecorder.Traces()
}

func pickLatestFinalAssistantContent(current string, resp *model.Response) string {
	if resp == nil {
		return current
	}

	for _, choice := range resp.Choices {
		if content, ok := finalAssistantContentFromMessage(choice.Message); ok {
			current = content
		}
		if content, ok := finalAssistantContentFromMessage(choice.Delta); ok {
			if current == "" {
				current = content
			} else {
				current += content
			}
		}
	}
	return current
}

func finalAssistantContentFromMessage(msg model.Message) (string, bool) {
	if len(msg.ToolCalls) > 0 || msg.ToolID != "" {
		return "", false
	}
	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return "", false
	}
	if msg.Role != "" && msg.Role != model.RoleAssistant {
		return "", false
	}
	return content, true
}

func recordToolCalls(
	resp *model.Response,
	toolStats *ToolCallStats,
	seenToolCalls map[string]struct{},
) {
	if resp == nil {
		return
	}
	for _, choice := range resp.Choices {
		for _, tc := range choice.Message.ToolCalls {
			recordToolCall(tc, toolStats, seenToolCalls)
		}
		for _, tc := range choice.Delta.ToolCalls {
			recordToolCall(tc, toolStats, seenToolCalls)
		}
	}
}

type toolTraceRecorder struct {
	traces    []*ToolTrace
	indexByID map[string]int
}

func newToolTraceRecorder() *toolTraceRecorder {
	return &toolTraceRecorder{
		indexByID: make(map[string]int),
	}
}

func (r *toolTraceRecorder) Traces() []ToolTrace {
	if r == nil || len(r.traces) == 0 {
		return nil
	}
	out := make([]ToolTrace, 0, len(r.traces))
	for _, trace := range r.traces {
		if trace == nil {
			continue
		}
		out = append(out, *trace)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (r *toolTraceRecorder) RecordResponse(resp *model.Response) {
	if r == nil || resp == nil {
		return
	}
	for _, choice := range resp.Choices {
		for _, tc := range choice.Message.ToolCalls {
			r.recordToolCall(tc)
		}
		for _, tc := range choice.Delta.ToolCalls {
			r.recordToolCall(tc)
		}
		r.recordToolMessage(choice.Message)
		r.recordToolMessage(choice.Delta)
	}
}

func (r *toolTraceRecorder) recordToolCall(tc model.ToolCall) {
	name := strings.TrimSpace(tc.Function.Name)
	args := strings.TrimSpace(string(tc.Function.Arguments))
	if name == "" && strings.TrimSpace(tc.ID) == "" && args == "" {
		return
	}
	trace := r.ensureTrace(tc.ID, name)
	if trace.Name == "" {
		trace.Name = name
	}
	if trace.Arguments == "" {
		trace.Arguments = args
	}
}

func (r *toolTraceRecorder) recordToolMessage(msg model.Message) {
	toolID := strings.TrimSpace(msg.ToolID)
	toolName := strings.TrimSpace(msg.ToolName)
	content := strings.TrimSpace(msg.Content)
	if toolID == "" && toolName == "" {
		return
	}
	trace := r.ensureTrace(toolID, toolName)
	if trace.Name == "" {
		trace.Name = toolName
	}
	if content == "" {
		return
	}
	if trace.Response == "" {
		trace.Response = content
		return
	}
	trace.Response += content
}

func (r *toolTraceRecorder) ensureTrace(id, name string) *ToolTrace {
	id = strings.TrimSpace(id)
	if id != "" {
		if idx, ok := r.indexByID[id]; ok {
			return r.traces[idx]
		}
	}

	trace := &ToolTrace{
		ID:   id,
		Name: strings.TrimSpace(name),
	}
	if id != "" {
		r.indexByID[id] = len(r.traces)
	}
	r.traces = append(r.traces, trace)
	return trace
}

func recordToolCall(
	tc model.ToolCall,
	toolStats *ToolCallStats,
	seenToolCalls map[string]struct{},
) {
	name := strings.TrimSpace(tc.Function.Name)
	if name == "" {
		return
	}
	if tc.ID != "" {
		if _, ok := seenToolCalls[tc.ID]; ok {
			return
		}
		seenToolCalls[tc.ID] = struct{}{}
	}
	toolStats.increment(name)
}

func firstNonNil(values ...any) any {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func asInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int32:
		return int(x)
	case int64:
		return int(x)
	case float64:
		return int(x)
	case float32:
		return int(x)
	case string:
		x = strings.TrimSpace(x)
		if x == "" {
			return 0
		}
		i, err := strconv.Atoi(x)
		if err != nil {
			return 0
		}
		return i
	case json.Number:
		i, err := x.Int64()
		if err != nil {
			return 0
		}
		return int(i)
	default:
		return 0
	}
}

func asFloat64(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

func asStringSlice(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func asFloat64Slice(v any) []float64 {
	switch x := v.(type) {
	case []float64:
		return x
	case []any:
		out := make([]float64, 0, len(x))
		for _, item := range x {
			out = append(out, asFloat64(item))
		}
		return out
	default:
		return nil
	}
}

func intPtr(i int) *int { return &i }

func float64Ptr(v float64) *float64 { return &v }

// truncateStr truncates a string to maxLen characters, replacing newlines.
func truncateStr(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
