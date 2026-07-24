// Package sessionaccess provides the 3 read-only session retrieval tools
// assembled for the spirit agent (M71). Each tool is a thin shell: input
// validation → biz usecase delegation (with the caller's agent ID bound at
// assembly time) → result mapping. Access is rate-limited (20/min) and audited.
package sessionaccess

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Searcher is the biz-layer port backing the sessionaccess tools.
// *biz.SessionSearchUsecase satisfies it.
type Searcher interface {
	SearchMessages(ctx context.Context, callerAgentID, keyword, agentID string, limit int) ([]biz.GlobalMessageHit, error)
	ListAgentSessions(ctx context.Context, callerAgentID, agentID string, limit int) ([]biz.SessionMeta, error)
	ReadSessionHistory(ctx context.Context, callerAgentID, sessionID, beforeMessageID string, limit, maxChars int) ([]biz.HistoryMessage, bool, error)
}

// RegisterAll creates the 3 sessionaccess tools bound to the caller's identity.
func RegisterAll(s Searcher, callerAgentID string, lg loggateway.Logger) []trpctool.Tool {
	if s == nil || strings.TrimSpace(callerAgentID) == "" {
		return nil
	}
	return []trpctool.Tool{
		&searchMessagesTool{s: s, callerID: callerAgentID, lg: lg},
		&listAgentSessionsTool{s: s, callerID: callerAgentID, lg: lg},
		&readSessionHistoryTool{s: s, callerID: callerAgentID, lg: lg},
	}
}

// ---------------------------------------------------------------------------
// search_messages
// ---------------------------------------------------------------------------

type searchMessagesTool struct {
	s        Searcher
	callerID string
	lg       loggateway.Logger
}

type searchMessagesInput struct {
	Query   string `json:"query" jsonschema:"description=检索关键词（匹配会话中的用户提问与 agent 回复内容）,required"`
	AgentID string `json:"agent_id" jsonschema:"description=可选：限定某个 agent 的会话"`
	Limit   int    `json:"limit" jsonschema:"description=返回条数，默认 20，上限 50"`
}

type searchMessagesOutput struct {
	Hits  []biz.GlobalMessageHit `json:"hits"`
	Count int                    `json:"count"`
}

func (t *searchMessagesTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "search_messages",
		Description: "跨会话检索消息内容（用户提问与 agent 回复）。用于精灵了解全工作区" +
			"的历史工作记录、查找特定主题的相关讨论。只读访问，限流 20 次/分钟，全程审计。",
		InputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{"query"},
			Properties: map[string]*trpctool.Schema{
				"query":    {Type: "string", Description: "检索关键词。"},
				"agent_id": {Type: "string", Description: "可选：限定某个 agent 的会话。"},
				"limit":    {Type: "integer", Description: "返回条数，默认 20，上限 50。"},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "检索命中列表（按时间倒序）。",
			Required:    []string{"hits", "count"},
			Properties: map[string]*trpctool.Schema{
				"hits":  {Type: "array", Description: "命中列表（session_id/kind/snippet/started_at）。"},
				"count": {Type: "integer", Description: "命中总数。"},
			},
		},
	}
}

func (t *searchMessagesTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	var in searchMessagesInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}
	hits, err := t.s.SearchMessages(ctx, t.callerID, in.Query, in.AgentID, in.Limit)
	if err != nil {
		t.logWarn("search_messages", err)
		return nil, err
	}
	return searchMessagesOutput{Hits: hits, Count: len(hits)}, nil
}

func (t *searchMessagesTool) logWarn(step string, err error) {
	if t.lg != nil {
		t.lg.Warn("sessionaccess 工具调用失败", loggateway.StepID("tool."+step), loggateway.Err(err))
	}
}

// ---------------------------------------------------------------------------
// list_agent_sessions
// ---------------------------------------------------------------------------

type listAgentSessionsTool struct {
	s        Searcher
	callerID string
	lg       loggateway.Logger
}

type listAgentSessionsInput struct {
	AgentID string `json:"agent_id" jsonschema:"description=可选：限定某个 agent；缺省列出全工作区最近会话"`
	Limit   int    `json:"limit" jsonschema:"description=返回条数，默认 20，上限 50"`
}

type listAgentSessionsOutput struct {
	Sessions []biz.SessionMeta `json:"sessions"`
	Count    int               `json:"count"`
}

func (t *listAgentSessionsTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "list_agent_sessions",
		Description: "列出会话元数据（标题/所属 agent/消息数/状态/更新时间），按更新时间倒序。" +
			"用于定位某个员工最近的工作会话，再配合 read_session_history 查看详情。只读访问，限流 20 次/分钟。",
		InputSchema: &trpctool.Schema{
			Type: "object",
			Properties: map[string]*trpctool.Schema{
				"agent_id": {Type: "string", Description: "可选：限定某个 agent；缺省列出全工作区最近会话。"},
				"limit":    {Type: "integer", Description: "返回条数，默认 20，上限 50。"},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "会话元数据列表。",
			Required:    []string{"sessions", "count"},
			Properties: map[string]*trpctool.Schema{
				"sessions": {Type: "array", Description: "会话列表（id/title/agent_id/message_count/status/updated_at）。"},
				"count":    {Type: "integer", Description: "会话总数。"},
			},
		},
	}
}

func (t *listAgentSessionsTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	var in listAgentSessionsInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}
	sessions, err := t.s.ListAgentSessions(ctx, t.callerID, in.AgentID, in.Limit)
	if err != nil {
		if t.lg != nil {
			t.lg.Warn("sessionaccess 工具调用失败", loggateway.StepID("tool.list_agent_sessions"), loggateway.Err(err))
		}
		return nil, err
	}
	return listAgentSessionsOutput{Sessions: sessions, Count: len(sessions)}, nil
}

// ---------------------------------------------------------------------------
// read_session_history
// ---------------------------------------------------------------------------

type readSessionHistoryTool struct {
	s        Searcher
	callerID string
	lg       loggateway.Logger
}

type readSessionHistoryInput struct {
	SessionID       string `json:"session_id" jsonschema:"description=目标会话 ID,required"`
	BeforeMessageID string `json:"before_message_id" jsonschema:"description=可选：只取该消息之前的历史（向前翻页）"`
	Limit           int    `json:"limit" jsonschema:"description=返回最近 N 条，默认 50，上限 200"`
	MaxChars        int    `json:"max_chars" jsonschema:"description=返回内容的最大字符数，默认 50000，上限 200000"`
}

type readSessionHistoryOutput struct {
	Messages  []biz.HistoryMessage `json:"messages"`
	Count     int                  `json:"count"`
	Truncated bool                 `json:"truncated"`
}

func (t *readSessionHistoryTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "read_session_history",
		Description: "读取一个会话的聊天消息历史（role/content/时间），默认取最近 50 条。" +
			"内容超过 max_chars 时截断并附 truncated 标记；可用 before_message_id 向前翻页。" +
			"只读访问，限流 20 次/分钟，全程审计。",
		InputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{"session_id"},
			Properties: map[string]*trpctool.Schema{
				"session_id":        {Type: "string", Description: "目标会话 ID。"},
				"before_message_id": {Type: "string", Description: "可选：只取该消息之前的历史（向前翻页）。"},
				"limit":             {Type: "integer", Description: "返回最近 N 条，默认 50，上限 200。"},
				"max_chars":         {Type: "integer", Description: "返回内容的最大字符数，默认 50000，上限 200000。"},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "消息历史（按时间正序）。",
			Required:    []string{"messages", "count", "truncated"},
			Properties: map[string]*trpctool.Schema{
				"messages":  {Type: "array", Description: "消息列表（id/role/content/created_at）。"},
				"count":     {Type: "integer", Description: "消息总数。"},
				"truncated": {Type: "boolean", Description: "内容是否被截断到 max_chars。"},
			},
		},
	}
}

func (t *readSessionHistoryTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	var in readSessionHistoryInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}
	if strings.TrimSpace(in.SessionID) == "" {
		return nil, errors.New("session_id 为必填项")
	}
	msgs, truncated, err := t.s.ReadSessionHistory(ctx, t.callerID, in.SessionID, in.BeforeMessageID, in.Limit, in.MaxChars)
	if err != nil {
		if t.lg != nil {
			t.lg.Warn("sessionaccess 工具调用失败", loggateway.StepID("tool.read_session_history"), loggateway.Err(err))
		}
		return nil, err
	}
	return readSessionHistoryOutput{Messages: msgs, Count: len(msgs), Truncated: truncated}, nil
}

// --- interface guards ---

var (
	_ trpctool.Tool         = (*searchMessagesTool)(nil)
	_ trpctool.CallableTool = (*searchMessagesTool)(nil)
	_ trpctool.Tool         = (*listAgentSessionsTool)(nil)
	_ trpctool.CallableTool = (*listAgentSessionsTool)(nil)
	_ trpctool.Tool         = (*readSessionHistoryTool)(nil)
	_ trpctool.CallableTool = (*readSessionHistoryTool)(nil)
)
