// Package openaicompat 实现 OpenAI 兼容 chat/completions 上游客户端：
// 流式 SSE + tool_calls 按 index 增量聚合 + DeepSeek 非流式回退档。
// vendor 差异收敛在本文件的 provider 分支（设计 §F），禁止外溢到 acpshim 通用层。
package openaicompat

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"aranea-agents/internal/agentbridge/acpshim"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// Provider 标识 vendor 分支（方言收敛点）。
type Provider int

const (
	ProviderDeepseek Provider = iota
	ProviderGrok
)

// String 日志标识。
func (p Provider) String() string {
	switch p {
	case ProviderDeepseek:
		return "deepseek"
	case ProviderGrok:
		return "grok"
	default:
		return "unknown"
	}
}

// Client 是 OpenAI 兼容上游客户端，实现 acpshim.LLMClient。
type Client struct {
	baseURL string // 例：https://api.deepseek.com / https://api.x.ai/v1
	apiKey  string
	provider Provider

	hc *http.Client
	lg loggateway.Logger

	// fallbackNonStream：流式 tool_calls 异常（HTTP 错/解码错）且尚未交付
	// 任何增量时，回退非流式重发一次（设计 §F，DeepSeek 档；Grok 关闭）。
	fallbackNonStream bool
}

// New 构造客户端。hc 为 nil 时用默认 http.Client（无全局超时——流式长连接，
// 取消全走 ctx；响应头等待 30s 由 Transport 层管不了，依赖 ctx/服务器）。
func New(baseURL, apiKey string, provider Provider, hc *http.Client, lg loggateway.Logger) *Client {
	if hc == nil {
		hc = &http.Client{}
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &Client{
		baseURL:           strings.TrimRight(baseURL, "/"),
		apiKey:            apiKey,
		provider:          provider,
		hc:                hc,
		lg:                lg.With(loggateway.Str("component", "openaicompat"), loggateway.Str("vendor", provider.String())),
		fallbackNonStream: provider == ProviderDeepseek,
	}
}

// Chat 发起一轮对话，返回增量通道（acpshim.LLMClient 契约）。
// tool_calls 流式增量按 index 聚合并只在 Done 帧交付全集。
func (c *Client) Chat(ctx context.Context, req acpshim.ChatRequest) (<-chan acpshim.ChatDelta, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, errors.New("openaicompat: api key empty")
	}
	out := make(chan acpshim.ChatDelta, 16)
	safego.GoBackground("openaicompat.chat", func() {
		defer close(out)
		delivered := 0
		err := c.stream(ctx, req, out, &delivered)
		if err == nil {
			return
		}
		if ctx.Err() != nil {
			return // 取消路径：直接关通道，loop 侧判 errPromptCancelled
		}
		if c.fallbackNonStream && delivered == 0 {
			c.lg.Warn("openaicompat: stream failed before any delta, falling back to non-stream",
				loggateway.Err(err))
			if err2 := c.nonStream(ctx, req, out); err2 == nil {
				return
			} else {
				err = fmt.Errorf("stream: %v; non-stream fallback: %w", err, err2)
			}
		}
		out <- acpshim.ChatDelta{Err: err}
	})
	return out, nil
}

// ---------------------------------------------------------------------------
// 线型（OpenAI 兼容 wire 形态；与 acpshim 通用类型隔离）
// ---------------------------------------------------------------------------

type wireToolCall struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type wireMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type wireChatRequest struct {
	Model      string        `json:"model"`
	Messages   []wireMessage `json:"messages"`
	Tools      []wireTool    `json:"tools,omitempty"`
	ToolChoice string        `json:"tool_choice,omitempty"`
	Stream     bool          `json:"stream"`
}

type wireTool struct {
	Type     string `json:"type"`
	Function struct {
		Name           string          `json:"name"`
		Description    string          `json:"description"`
		ParametersJSON json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type wireUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// toWire 通用类型 → 线型。
func toWire(req acpshim.ChatRequest) wireChatRequest {
	w := wireChatRequest{Model: req.Model, ToolChoice: "auto"}
	for _, m := range req.Messages {
		wm := wireMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.ToolCalls {
			var wt wireToolCall
			wt.ID = tc.ID
			wt.Type = "function"
			wt.Function.Name = tc.Name
			wt.Function.Arguments = tc.Arguments
			wm.ToolCalls = append(wm.ToolCalls, wt)
		}
		w.Messages = append(w.Messages, wm)
	}
	for _, t := range req.Tools {
		var wt wireTool
		wt.Type = "function"
		wt.Function.Name = t.Name
		wt.Function.Description = t.Description
		wt.Function.ParametersJSON = t.ParametersJSON
		w.Tools = append(w.Tools, wt)
	}
	return w
}

// ---------------------------------------------------------------------------
// 流式（SSE）
// ---------------------------------------------------------------------------

type streamChunkToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Role             string                `json:"role"`
			Content          string                `json:"content"`
			ReasoningContent string                `json:"reasoning_content"`
			ToolCalls        []streamChunkToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *wireUsage `json:"usage"`
}

// toolAgg 单 index 聚合槽：id/name 只在首帧落，后续帧不覆盖（fixture 假设 2）。
type toolAgg struct {
	id   string
	name string
	args strings.Builder
}

func (c *Client) stream(ctx context.Context, req acpshim.ChatRequest, out chan<- acpshim.ChatDelta, delivered *int) error {
	body := toWire(req)
	body.Stream = true
	resp, err := c.do(ctx, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	aggs := map[int]*toolAgg{}
	finishReason := ""
	var usage *wireUsage
	br := bufio.NewReaderSize(resp.Body, 64*1024)
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			if stop, perr := c.handleSSELine(line, aggs, &finishReason, &usage, out, delivered); perr != nil {
				return perr
			} else if stop {
				break
			}
		}
		if err != nil {
			if err == io.EOF {
				break // 容忍无 [DONE] 提前收流
			}
			return fmt.Errorf("openaicompat: read stream: %w", err)
		}
	}

	final := acpshim.ChatDelta{Done: true, FinishReason: finishReason}
	if usage != nil {
		final.Usage = &acpshim.Usage{PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens}
	}
	final.ToolCalls = collectAggs(aggs)
	select {
	case out <- final:
		(*delivered)++
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// handleSSELine 处理一行 SSE；stop=true 表示 [DONE]。
func (c *Client) handleSSELine(line string, aggs map[int]*toolAgg, finishReason *string, usage **wireUsage, out chan<- acpshim.ChatDelta, delivered *int) (bool, error) {
	line = strings.TrimRight(line, "\r\n")
	if line == "" || strings.HasPrefix(line, ":") {
		return false, nil // 空行/注释行
	}
	if !strings.HasPrefix(line, "data:") {
		return false, nil // 容错：非 data 行跳过不断流
	}
	payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if payload == "[DONE]" {
		return true, nil
	}
	var chunk streamChunk
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return false, fmt.Errorf("openaicompat: decode chunk: %w (%s)", err, truncate(payload, 200))
	}
	if chunk.Usage != nil {
		*usage = chunk.Usage
	}
	for _, ch := range chunk.Choices {
		if ch.Delta.Content != "" {
			out <- acpshim.ChatDelta{Text: ch.Delta.Content}
			(*delivered)++
		}
		if ch.Delta.ReasoningContent != "" {
			out <- acpshim.ChatDelta{Reasoning: ch.Delta.ReasoningContent}
			(*delivered)++
		}
		for _, tc := range ch.Delta.ToolCalls {
			agg := aggs[tc.Index]
			if agg == nil {
				agg = &toolAgg{}
				aggs[tc.Index] = agg
			}
			if tc.ID != "" {
				agg.id = tc.ID // 首帧落 id；后续帧 id 缺席/null 不覆盖
			}
			if tc.Function.Name != "" {
				agg.name = tc.Function.Name
			}
			agg.args.WriteString(tc.Function.Arguments)
		}
		if ch.FinishReason != nil && *ch.FinishReason != "" {
			*finishReason = *ch.FinishReason
		}
	}
	return false, nil
}

// collectAggs 按 index 升序交付聚合完成的工具调用全集。
func collectAggs(aggs map[int]*toolAgg) []acpshim.ToolCallRequest {
	if len(aggs) == 0 {
		return nil
	}
	idxs := make([]int, 0, len(aggs))
	for i := range aggs {
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	out := make([]acpshim.ToolCallRequest, 0, len(idxs))
	for _, i := range idxs {
		a := aggs[i]
		out = append(out, acpshim.ToolCallRequest{
			Index: i, ID: a.id, Name: a.name, Arguments: a.args.String(),
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// 非流式回退档
// ---------------------------------------------------------------------------

type nonStreamResponse struct {
	Choices []struct {
		Message struct {
			Role             string        `json:"role"`
			Content          string        `json:"content"`
			ReasoningContent string        `json:"reasoning_content"`
			ToolCalls        []wireToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *wireUsage `json:"usage"`
}

func (c *Client) nonStream(ctx context.Context, req acpshim.ChatRequest, out chan<- acpshim.ChatDelta) error {
	body := toWire(req)
	body.Stream = false
	resp, err := c.do(ctx, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("openaicompat: read body: %w", err)
	}
	var nr nonStreamResponse
	if err := json.Unmarshal(data, &nr); err != nil {
		return fmt.Errorf("openaicompat: decode response: %w", err)
	}
	if len(nr.Choices) == 0 {
		return errors.New("openaicompat: empty choices")
	}
	ch := nr.Choices[0]
	if ch.Message.Content != "" {
		out <- acpshim.ChatDelta{Text: ch.Message.Content}
	}
	final := acpshim.ChatDelta{Done: true, FinishReason: ch.FinishReason}
	for i, tc := range ch.Message.ToolCalls {
		final.ToolCalls = append(final.ToolCalls, acpshim.ToolCallRequest{
			Index: i, ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments,
		})
	}
	if nr.Usage != nil {
		final.Usage = &acpshim.Usage{PromptTokens: nr.Usage.PromptTokens, CompletionTokens: nr.Usage.CompletionTokens}
	}
	out <- final
	return nil
}

// ---------------------------------------------------------------------------
// HTTP 层
// ---------------------------------------------------------------------------

func (c *Client) do(ctx context.Context, body wireChatRequest) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body.Stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openaicompat: http: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("openaicompat: vendor http %d: %s", resp.StatusCode, truncate(string(snippet), 400))
	}
	return resp, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
