package deliverable

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// --- read_upstream_deliverable (P2 产物引用化) ---

// UpstreamDeliverableReader is the biz-layer port backing the tool.
// *biz.SpiritTeamUsecase satisfies it (via biz.SpiritTeamController).
// readerSessionID identifies the calling team's main session so the biz layer
// can run the runtime contract check against its InputContract (Phase B).
type UpstreamDeliverableReader interface {
	ReadUpstreamDeliverable(ctx context.Context, readerSessionID, teamID string, maxChars int) (biz.UpstreamDeliverableContent, error)
	// ReadUpstreamDeliverableKey fetches ONE payload entry by key (envelope v2
	// artifacts): downstream teams with a long-form deliverable (e.g. an
	// article to publish) read only the contracted payload instead of the
	// whole concatenated deliverable.
	ReadUpstreamDeliverableKey(ctx context.Context, readerSessionID, teamID, key string, maxChars int) (biz.UpstreamDeliverableContent, error)
}

// ReadUpstreamDeliverableTool lets a downstream team member retrieve the FULL
// text of an upstream team's deliverable on demand. The DAG injection prefix
// only carries a truncated summary (P2 DeliverableRef envelope); when the
// summary is insufficient, the agent calls this tool instead of bloating the
// prompt with every upstream full text.
type ReadUpstreamDeliverableTool struct {
	reader UpstreamDeliverableReader
	lg     loggateway.Logger
}

// NewReadUpstreamDeliverableTool creates the read_upstream_deliverable tool.
func NewReadUpstreamDeliverableTool(reader UpstreamDeliverableReader, lg loggateway.Logger) *ReadUpstreamDeliverableTool {
	return &ReadUpstreamDeliverableTool{reader: reader, lg: lg}
}

type readUpstreamDeliverableInput struct {
	TeamID   string `json:"team_id" jsonschema:"description=上游团队ID（注入前缀中 read_upstream_deliverable 指引给出的 team_id）,required"`
	Key      string `json:"key" jsonschema:"description=可选：只取某个载荷条目（注入前缀交付物清单中给出的 key，如长文场景的文章载荷）；不传则返回交付物全文"`
	MaxChars int    `json:"max_chars" jsonschema:"description=返回内容的最大字符数，默认 50000，上限 200000"`
}

type readUpstreamDeliverableOutput struct {
	Content   string `json:"content"`
	SizeChars int    `json:"size_chars"`
	Truncated bool   `json:"truncated"`
	TeamID    string `json:"team_id"`
	SessionID string `json:"session_id"`
	Key       string `json:"key,omitempty"`
}

// Declaration returns the tool metadata.
func (t *ReadUpstreamDeliverableTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "read_upstream_deliverable",
		Description: "读取上游团队交付物的完整内容。当上游交付物注入前缀中的摘要被截断" +
			"（附有 read_upstream_deliverable 指引）且摘要不足以完成当前任务时，" +
			"使用此工具按需获取全文。传入 key 可只取交付物清单中的某个载荷条目" +
			"（如长文场景契约要求的文章载荷）；不传 key 返回交付物全文。" +
			"仅对已完成的上游团队有效。",
		InputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{"team_id"},
			Properties: map[string]*trpctool.Schema{
				"team_id": {
					Type:        "string",
					Description: "上游团队的唯一标识符（注入前缀指引中给出）。",
				},
				"key": {
					Type:        "string",
					Description: "可选：只取某个载荷条目（注入前缀交付物清单中给出的 key）。summary/cognition 为保留 key，不可读取。",
				},
				"max_chars": {
					Type:        "integer",
					Description: fmt.Sprintf("返回内容的最大字符数，默认 %d，上限 %d。", biz.DefaultUpstreamDeliverableMaxChars, biz.MaxUpstreamDeliverableChars),
				},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "上游团队交付物内容（可能被截断到 max_chars）。",
			Required:    []string{"content", "size_chars", "truncated"},
			Properties: map[string]*trpctool.Schema{
				"content":    {Type: "string", Description: "交付物内容（截断到 max_chars 时末尾附截断标记）。"},
				"size_chars": {Type: "integer", Description: "全文总字符数（截断前）。"},
				"truncated":  {Type: "boolean", Description: "内容是否被截断到 max_chars。"},
				"team_id":    {Type: "string", Description: "上游团队 ID。"},
				"session_id": {Type: "string", Description: "上游团队主会话 ID。"},
				"key":        {Type: "string", Description: "本次读取的载荷 key（仅按 key 读取时返回）。"},
			},
		},
	}
}

// Call validates input, delegates to the biz reader, and maps the result.
func (t *ReadUpstreamDeliverableTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if t == nil || t.reader == nil {
		return nil, errors.New("read_upstream_deliverable is not configured")
	}
	var in readUpstreamDeliverableInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}
	teamID := strings.TrimSpace(in.TeamID)
	if teamID == "" {
		return nil, errors.New("team_id 为必填项")
	}
	maxChars := in.MaxChars
	if maxChars <= 0 {
		maxChars = biz.DefaultUpstreamDeliverableMaxChars
	}
	if maxChars > biz.MaxUpstreamDeliverableChars {
		maxChars = biz.MaxUpstreamDeliverableChars
	}

	key := strings.TrimSpace(in.Key)
	var out biz.UpstreamDeliverableContent
	var err error
	if key != "" {
		out, err = t.reader.ReadUpstreamDeliverableKey(ctx, readerSessionIDFromCtx(ctx), teamID, key, maxChars)
	} else {
		out, err = t.reader.ReadUpstreamDeliverable(ctx, readerSessionIDFromCtx(ctx), teamID, maxChars)
	}
	if err != nil {
		if t.lg != nil {
			t.lg.Warn("读取上游交付物失败",
				loggateway.StepID("tool.read_upstream_deliverable"),
				loggateway.Str("team_id", teamID),
				loggateway.Str("key", key),
				loggateway.Err(err),
			)
		}
		return nil, err
	}
	return readUpstreamDeliverableOutput{
		Content:   out.Content,
		SizeChars: out.SizeChars,
		Truncated: out.Truncated,
		TeamID:    out.TeamID,
		SessionID: out.SessionID,
		Key:       key,
	}, nil
}

// readerSessionIDFromCtx extracts the calling agent's session ID from the
// invocation context (empty when the tool is invoked outside an agent run,
// e.g. CLI — the biz layer then skips the runtime contract check).
func readerSessionIDFromCtx(ctx context.Context) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return ""
	}
	return strings.TrimSpace(inv.Session.ID)
}

// --- interface guards ---

var (
	_ trpctool.Tool         = (*ReadUpstreamDeliverableTool)(nil)
	_ trpctool.CallableTool = (*ReadUpstreamDeliverableTool)(nil)
)
