// Package mail 实现 agent 侧邮件四动词（E 阶段 W1d，需求文档 §1.3「agent
// 触达」）：
//
//   - mail_list    — 读本会话属主的邮件列表（只读，低风险，种子默认开）。
//   - mail_read    — 读单封邮件正文（只读，低风险）。
//   - mail_search  — 本人邮件关键词检索（只读，低风险）。
//   - mail_to_task — 邮件转个人任务（中风险，种子默认关按 agent 授予；幂等——
//     task_ref 非空返回既有引用，任务含正文与回链）。
//
// 身份解析在 service 层适配口完成：工具只带 GateSessionID 注入的 sessionID，
// user/workspace 由适配器查会话落位（agent 不可自报身份）。
//
// 装配：tools 表种子 mail=enabled=true（只读三动词默认开）、mail_task=
// enabled=false（转任务按 agent 授予）；Service 为 nil 时工具集被裁剪
// （诚实降级，不伪造空表）。
package mail

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz/decision"
	"aranea-agents/pkg/apierror"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// MessageView 是邮件的工具结果投影（仅展示字段；body 仅 read 动词返回）。
type MessageView struct {
	ID             string `json:"id"`
	AccountID      string `json:"account_id,omitempty"`
	Subject        string `json:"subject"`
	Sender         string `json:"sender"`
	Snippet        string `json:"snippet"`
	ReceivedAt     string `json:"received_at,omitempty"` // RFC3339
	HasAttachments bool   `json:"has_attachments"`
	TaskRef        string `json:"task_ref,omitempty"`
}

// MessageDetailView 单封邮件详情（read 动词）。
type MessageDetailView struct {
	MessageView
	Body string `json:"body"`
}

// Service 是邮件工具的服务端窄口（由 service 层 officeMailTool 适配器满足：
// session→user 解析 + office.MailMessageUsecase 调用）。
// Stability:evolving
type Service interface {
	// ListMyMessages 返回会话属主的邮件分页（accountID 空=全账号聚合），
	// 按收件时间倒序。
	ListMyMessages(ctx context.Context, sessionID, accountID string, page, pageSize int) ([]MessageView, int64, error)
	// ReadMyMessage 读单封邮件全文（属主隔离在服务端）。
	ReadMyMessage(ctx context.Context, sessionID, messageID string) (*MessageDetailView, error)
	// SearchMyMessages 本人邮件关键词检索（subject/sender/body），按收件时间
	// 倒序限量。
	SearchMyMessages(ctx context.Context, sessionID, query string, limit int) ([]MessageView, error)
	// ToTask 邮件转个人任务（幂等：task_ref 非空返回既有引用）。
	ToTask(ctx context.Context, sessionID, messageID, titleOverride string) (string, error)
}

// NewToolset 构造只读三动词工具集；svc 为 nil 返回 nil（未装配邮件模块时
// 不暴露）。
func NewToolset(svc Service) []trpctool.CallableTool {
	if svc == nil {
		return nil
	}
	return []trpctool.CallableTool{
		&listTool{svc: svc},
		&readTool{svc: svc},
		&searchTool{svc: svc},
	}
}

// NewTaskToolset 构造转任务动词工具集（中风险，独立授权键 mail_task）；
// svc 为 nil 返回 nil。
func NewTaskToolset(svc Service) []trpctool.CallableTool {
	if svc == nil {
		return nil
	}
	return []trpctool.CallableTool{
		&toTaskTool{svc: svc},
	}
}

// ---------- mail_list ----------

type listTool struct{ svc Service }

func (t *listTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "mail_list",
		Description: "List the current user's office mailbox messages, newest first. " +
			"Optional account_id scopes to one mail account. Read-only.",
		InputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "List mailbox messages",
			Properties: map[string]*trpctool.Schema{
				"account_id": {Type: "string", Description: "Optional mail account id filter"},
				"page":       {Type: "integer", Description: "Page number (default 1)"},
				"page_size":  {Type: "integer", Description: "Page size (default 20, max 50)"},
			},
		},
	}
}

func (t *listTool) Call(ctx context.Context, args []byte) (any, error) {
	var in struct {
		AccountID string `json:"account_id"`
		Page      int    `json:"page"`
		PageSize  int    `json:"page_size"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, apierror.BadRequest(apierror.DomainTool, "mail_list: invalid args: "+err.Error())
	}
	if in.Page <= 0 {
		in.Page = 1
	}
	if in.PageSize <= 0 {
		in.PageSize = 20
	}
	if in.PageSize > 50 {
		in.PageSize = 50
	}
	sessionID := decision.GateSessionIDFromContext(ctx)
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainTool, "mail_list: session context unavailable")
	}
	items, total, err := t.svc.ListMyMessages(ctx, sessionID, strings.TrimSpace(in.AccountID), in.Page, in.PageSize)
	if err != nil {
		return nil, err
	}
	return map[string]any{"messages": items, "total": total}, nil
}

func (t *listTool) StreamableCall(ctx context.Context, _ []byte) (*trpctool.StreamReader, error) {
	return nil, apierror.BadRequest(apierror.DomainTool, "mail_list: not streamable")
}

// ---------- mail_read ----------

type readTool struct{ svc Service }

func (t *readTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "mail_read",
		Description: "Read one mailbox message in full (subject, sender, body). " +
			"Only messages owned by the current user are accessible. Read-only.",
		InputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "Read one message",
			Properties: map[string]*trpctool.Schema{
				"message_id": {Type: "string", Description: "Message id (from mail_list/mail_search)"},
			},
			Required: []string{"message_id"},
		},
	}
}

func (t *readTool) Call(ctx context.Context, args []byte) (any, error) {
	var in struct {
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, apierror.BadRequest(apierror.DomainTool, "mail_read: invalid args: "+err.Error())
	}
	if strings.TrimSpace(in.MessageID) == "" {
		return nil, apierror.BadRequest(apierror.DomainTool, "mail_read: message_id is required")
	}
	sessionID := decision.GateSessionIDFromContext(ctx)
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainTool, "mail_read: session context unavailable")
	}
	msg, err := t.svc.ReadMyMessage(ctx, sessionID, strings.TrimSpace(in.MessageID))
	if err != nil {
		return nil, err
	}
	return msg, nil
}

func (t *readTool) StreamableCall(ctx context.Context, _ []byte) (*trpctool.StreamReader, error) {
	return nil, apierror.BadRequest(apierror.DomainTool, "mail_read: not streamable")
}

// ---------- mail_search ----------

type searchTool struct{ svc Service }

func (t *searchTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "mail_search",
		Description: "Search the current user's mailbox by keyword over subject, sender and body. " +
			"Newest first. Read-only.",
		InputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "Search mailbox messages",
			Properties: map[string]*trpctool.Schema{
				"query": {Type: "string", Description: "Keyword to match in subject/sender/body"},
				"limit": {Type: "integer", Description: "Max results (default 10, max 50)"},
			},
			Required: []string{"query"},
		},
	}
}

func (t *searchTool) Call(ctx context.Context, args []byte) (any, error) {
	var in struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, apierror.BadRequest(apierror.DomainTool, "mail_search: invalid args: "+err.Error())
	}
	if strings.TrimSpace(in.Query) == "" {
		return nil, apierror.BadRequest(apierror.DomainTool, "mail_search: query is required")
	}
	if in.Limit <= 0 {
		in.Limit = 10
	}
	if in.Limit > 50 {
		in.Limit = 50
	}
	sessionID := decision.GateSessionIDFromContext(ctx)
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainTool, "mail_search: session context unavailable")
	}
	items, err := t.svc.SearchMyMessages(ctx, sessionID, strings.TrimSpace(in.Query), in.Limit)
	if err != nil {
		return nil, err
	}
	return map[string]any{"messages": items, "count": len(items)}, nil
}

func (t *searchTool) StreamableCall(ctx context.Context, _ []byte) (*trpctool.StreamReader, error) {
	return nil, apierror.BadRequest(apierror.DomainTool, "mail_search: not streamable")
}

// ---------- mail_to_task ----------

type toTaskTool struct{ svc Service }

func (t *toTaskTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "mail_to_task",
		Description: "Convert a mailbox message into a personal task (title defaults to the subject, " +
			"body carries the message text and a backlink). Idempotent: converting the same message " +
			"again returns the existing task reference.",
		InputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "Convert message to task",
			Properties: map[string]*trpctool.Schema{
				"message_id":     {Type: "string", Description: "Message id (from mail_list/mail_search)"},
				"title_override": {Type: "string", Description: "Optional task title (default: mail subject)"},
			},
			Required: []string{"message_id"},
		},
	}
}

func (t *toTaskTool) Call(ctx context.Context, args []byte) (any, error) {
	var in struct {
		MessageID     string `json:"message_id"`
		TitleOverride string `json:"title_override"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, apierror.BadRequest(apierror.DomainTool, "mail_to_task: invalid args: "+err.Error())
	}
	if strings.TrimSpace(in.MessageID) == "" {
		return nil, apierror.BadRequest(apierror.DomainTool, "mail_to_task: message_id is required")
	}
	sessionID := decision.GateSessionIDFromContext(ctx)
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainTool, "mail_to_task: session context unavailable")
	}
	taskRef, err := t.svc.ToTask(ctx, sessionID, strings.TrimSpace(in.MessageID), strings.TrimSpace(in.TitleOverride))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"status":  "ok",
		"task_id": taskRef,
		"detail":  "task created (or reused when the message was already converted); the mail row carries task_ref",
	}, nil
}

func (t *toTaskTool) StreamableCall(ctx context.Context, _ []byte) (*trpctool.StreamReader, error) {
	return nil, apierror.BadRequest(apierror.DomainTool, "mail_to_task: not streamable")
}
