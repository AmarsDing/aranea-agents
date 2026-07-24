// Package deptmail provides the 4 mailbox tools assembled for department lead
// agents (M71): cross-department async messaging between dept leads. Each tool
// is a thin shell: input validation → biz usecase delegation (with the caller's
// agent ID bound at assembly time) → result mapping.
package deptmail

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

// Mailer is the biz-layer port backing the deptmail tools.
// *biz.DeptMailboxUsecase satisfies it.
type Mailer interface {
	SendMessage(ctx context.Context, fromAgentID, toDeptID, subject, body, refsJSON string) (biz.DeptLeadMessage, error)
	ListInbox(ctx context.Context, callerAgentID, status string, limit int) ([]biz.DeptLeadMessage, error)
	ReadMessage(ctx context.Context, callerAgentID, messageID string) (biz.DeptLeadMessage, error)
	ReplyMessage(ctx context.Context, callerAgentID, messageID, body string) (biz.DeptLeadMessage, error)
}

// RegisterAll creates the 4 deptmail tools bound to the caller's identity.
func RegisterAll(m Mailer, callerAgentID string, lg loggateway.Logger) []trpctool.Tool {
	if m == nil || strings.TrimSpace(callerAgentID) == "" {
		return nil
	}
	return []trpctool.Tool{
		&sendDeptMessageTool{m: m, callerID: callerAgentID, lg: lg},
		&listInboxTool{m: m, callerID: callerAgentID, lg: lg},
		&readMessageTool{m: m, callerID: callerAgentID, lg: lg},
		&replyMessageTool{m: m, callerID: callerAgentID, lg: lg},
	}
}

// messageView is the JSON projection of one mailbox message.
type messageView struct {
	ID         string `json:"id"`
	FromDeptID string `json:"from_dept_id"`
	ToDeptID   string `json:"to_dept_id"`
	Subject    string `json:"subject"`
	Body       string `json:"body,omitempty"`
	RefsJSON   string `json:"refs,omitempty"`
	Status     string `json:"status"`
	ReplyToID  string `json:"reply_to_id,omitempty"`
	CreatedAt  string `json:"created_at"`
	ReadAt     string `json:"read_at,omitempty"`
}

func toMessageView(msg biz.DeptLeadMessage, withBody bool) messageView {
	v := messageView{
		ID:         msg.ID,
		FromDeptID: msg.FromDeptID,
		ToDeptID:   msg.ToDeptID,
		Subject:    msg.Subject,
		RefsJSON:   msg.RefsJSON,
		Status:     msg.Status,
		ReplyToID:  msg.ReplyToID,
		CreatedAt:  msg.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if withBody {
		v.Body = msg.Body
	}
	if msg.ReadAt != nil {
		v.ReadAt = msg.ReadAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return v
}

// ---------------------------------------------------------------------------
// send_dept_message
// ---------------------------------------------------------------------------

type sendDeptMessageTool struct {
	m        Mailer
	callerID string
	lg       loggateway.Logger
}

type sendDeptMessageInput struct {
	ToDeptID string `json:"to_dept_id" jsonschema:"description=目标部门的节点 ID（组织架构中 level=department 的节点）,required"`
	Subject  string `json:"subject" jsonschema:"description=消息主题（≤200 字符）,required"`
	Body     string `json:"body" jsonschema:"description=消息正文（Markdown）,required"`
	Refs     string `json:"refs" jsonschema:"description=关联交付物引用（JSON 数组字符串，元素为 DeliverableRef 信封），可选"`
}

func (t *sendDeptMessageTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "send_dept_message",
		Description: "向另一个部门的主管发送异步消息（部门信箱）。消息落库后对方主管会被唤醒查收。" +
			"适用于跨部门协作请求、信息通报、交付物转交。不能发送给本部门。",
		InputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{"to_dept_id", "subject", "body"},
			Properties: map[string]*trpctool.Schema{
				"to_dept_id": {Type: "string", Description: "目标部门的节点 ID（组织架构中 level=department 的节点）。"},
				"subject":    {Type: "string", Description: "消息主题（≤200 字符）。"},
				"body":       {Type: "string", Description: "消息正文（Markdown）。"},
				"refs":       {Type: "string", Description: "关联交付物引用（JSON 数组字符串），可选。"},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "已发送的消息。",
			Required:    []string{"id", "status"},
			Properties: map[string]*trpctool.Schema{
				"id":     {Type: "string", Description: "消息 ID。"},
				"status": {Type: "string", Description: "消息状态（unread）。"},
			},
		},
	}
}

func (t *sendDeptMessageTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	var in sendDeptMessageInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}
	saved, err := t.m.SendMessage(ctx, t.callerID, in.ToDeptID, in.Subject, in.Body, in.Refs)
	if err != nil {
		t.logWarn("send_dept_message", err)
		return nil, err
	}
	return map[string]any{"id": saved.ID, "status": saved.Status}, nil
}

func (t *sendDeptMessageTool) logWarn(step string, err error) {
	if t.lg != nil {
		t.lg.Warn("deptmail 工具调用失败", loggateway.StepID("tool."+step), loggateway.Err(err))
	}
}

// ---------------------------------------------------------------------------
// list_inbox
// ---------------------------------------------------------------------------

type listInboxTool struct {
	m        Mailer
	callerID string
	lg       loggateway.Logger
}

type listInboxInput struct {
	Status string `json:"status" jsonschema:"description=状态过滤：unread/read/replied，默认全部"`
	Limit  int    `json:"limit" jsonschema:"description=返回条数，默认 20，上限 100"`
}

type listInboxOutput struct {
	Messages []messageView `json:"messages"`
	Count    int           `json:"count"`
}

func (t *listInboxTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "list_inbox",
		Description: "列出本人（部门主管）信箱收到的消息摘要（不含正文），按时间倒序。" +
			"收到【部门信箱】唤醒提示后，先用本工具查收未读消息。",
		InputSchema: &trpctool.Schema{
			Type: "object",
			Properties: map[string]*trpctool.Schema{
				"status": {Type: "string", Description: "状态过滤：unread/read/replied，默认全部。"},
				"limit":  {Type: "integer", Description: "返回条数，默认 20，上限 100。"},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "收件箱消息摘要列表。",
			Required:    []string{"messages", "count"},
			Properties: map[string]*trpctool.Schema{
				"messages": {Type: "array", Description: "消息摘要列表（id/from_dept_id/subject/status/created_at）。"},
				"count":    {Type: "integer", Description: "消息总数。"},
			},
		},
	}
}

func (t *listInboxTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	var in listInboxInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}
	items, err := t.m.ListInbox(ctx, t.callerID, in.Status, in.Limit)
	if err != nil {
		if t.lg != nil {
			t.lg.Warn("deptmail 工具调用失败", loggateway.StepID("tool.list_inbox"), loggateway.Err(err))
		}
		return nil, err
	}
	out := make([]messageView, 0, len(items))
	for _, msg := range items {
		out = append(out, toMessageView(msg, false))
	}
	return listInboxOutput{Messages: out, Count: len(out)}, nil
}

// ---------------------------------------------------------------------------
// read_message
// ---------------------------------------------------------------------------

type readMessageTool struct {
	m        Mailer
	callerID string
	lg       loggateway.Logger
}

type readMessageInput struct {
	MessageID string `json:"message_id" jsonschema:"description=消息 ID（list_inbox 返回的 id）,required"`
}

func (t *readMessageTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "read_message",
		Description: "读取信箱中一条消息的完整内容（含正文与交付物引用）。" +
			"属于本人收件且状态为 unread 时会自动标记为已读。",
		InputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{"message_id"},
			Properties: map[string]*trpctool.Schema{
				"message_id": {Type: "string", Description: "消息 ID（list_inbox 返回的 id）。"},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "消息全文。",
			Required:    []string{"id", "subject", "body", "status"},
			Properties: map[string]*trpctool.Schema{
				"id":      {Type: "string", Description: "消息 ID。"},
				"subject": {Type: "string", Description: "主题。"},
				"body":    {Type: "string", Description: "正文（Markdown）。"},
				"status":  {Type: "string", Description: "状态（unread/read/replied）。"},
				"refs":    {Type: "string", Description: "关联交付物引用（JSON 数组）。"},
			},
		},
	}
}

func (t *readMessageTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	var in readMessageInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}
	if strings.TrimSpace(in.MessageID) == "" {
		return nil, errors.New("message_id 为必填项")
	}
	msg, err := t.m.ReadMessage(ctx, t.callerID, in.MessageID)
	if err != nil {
		if t.lg != nil {
			t.lg.Warn("deptmail 工具调用失败", loggateway.StepID("tool.read_message"), loggateway.Err(err))
		}
		return nil, err
	}
	return toMessageView(msg, true), nil
}

// ---------------------------------------------------------------------------
// reply_message
// ---------------------------------------------------------------------------

type replyMessageTool struct {
	m        Mailer
	callerID string
	lg       loggateway.Logger
}

type replyMessageInput struct {
	MessageID string `json:"message_id" jsonschema:"description=被回复的消息 ID,required"`
	Body      string `json:"body" jsonschema:"description=回复正文（Markdown）,required"`
}

func (t *replyMessageTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "reply_message",
		Description: "回复收到的一条部门信箱消息，形成消息线程。原消息会标记为 replied，" +
			"对方主管会被唤醒查收回复。只能回复发给本人的消息。",
		InputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{"message_id", "body"},
			Properties: map[string]*trpctool.Schema{
				"message_id": {Type: "string", Description: "被回复的消息 ID。"},
				"body":       {Type: "string", Description: "回复正文（Markdown）。"},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "已发送的回复消息。",
			Required:    []string{"id", "status", "reply_to_id"},
			Properties: map[string]*trpctool.Schema{
				"id":          {Type: "string", Description: "回复消息 ID。"},
				"status":      {Type: "string", Description: "消息状态（unread）。"},
				"reply_to_id": {Type: "string", Description: "被回复的消息 ID。"},
			},
		},
	}
}

func (t *replyMessageTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	var in replyMessageInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}
	saved, err := t.m.ReplyMessage(ctx, t.callerID, in.MessageID, in.Body)
	if err != nil {
		if t.lg != nil {
			t.lg.Warn("deptmail 工具调用失败", loggateway.StepID("tool.reply_message"), loggateway.Err(err))
		}
		return nil, err
	}
	return map[string]any{"id": saved.ID, "status": saved.Status, "reply_to_id": saved.ReplyToID}, nil
}

// --- interface guards ---

var (
	_ trpctool.Tool         = (*sendDeptMessageTool)(nil)
	_ trpctool.CallableTool = (*sendDeptMessageTool)(nil)
	_ trpctool.Tool         = (*listInboxTool)(nil)
	_ trpctool.CallableTool = (*listInboxTool)(nil)
	_ trpctool.Tool         = (*readMessageTool)(nil)
	_ trpctool.CallableTool = (*readMessageTool)(nil)
	_ trpctool.Tool         = (*replyMessageTool)(nil)
	_ trpctool.CallableTool = (*replyMessageTool)(nil)
)
