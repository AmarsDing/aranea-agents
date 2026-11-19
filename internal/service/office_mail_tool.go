package service

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/office"
	"aranea-agents/internal/tools/mail"
	"aranea-agents/pkg/apierror"
)

// ============================================================================
// officeMailTool —— mail_list / mail_read / mail_search / mail_to_task 工具
// 窄口（E 阶段 W1d；tools/mail.Service ← office 用例 + session 解析）。
//
// 身份只从会话解析（session→user/workspace/agent），工具入参不含身份字段，
// agent 不可自报身份。
// ============================================================================

type officeMailTool struct {
	messages *office.MailMessageUsecase
	sessions *biz.SessionUsecase
}

var _ mail.Service = (*officeMailTool)(nil)

// ProvideOfficeMailTool 装配邮件工具窄口（wire 注入 ToolBridges.Mail）。
// 任一为 nil → nil（工具被裁剪，诚实降级）。
func ProvideOfficeMailTool(m *office.MailMessageUsecase, sessions *biz.SessionUsecase) mail.Service {
	if m == nil || sessions == nil {
		return nil
	}
	return &officeMailTool{messages: m, sessions: sessions}
}

func (a *officeMailTool) resolveUser(ctx context.Context, sessionID string) (string, error) {
	sess, err := a.sessions.Get(ctx, sessionID)
	if err != nil {
		return "", apierror.NotFound(office.DomainOffice, "session not found")
	}
	if sess.UserID == "" {
		return "", apierror.Internal(office.DomainOffice, "session has no owner")
	}
	return sess.UserID, nil
}

func mailMessageToView(m *office.MailMessage) mail.MessageView {
	v := mail.MessageView{
		ID:             m.ID,
		AccountID:      m.AccountID,
		Subject:        m.Subject,
		Sender:         m.Sender,
		Snippet:        m.Snippet,
		HasAttachments: m.HasAttachments,
		TaskRef:        m.TaskRef,
	}
	if m.ReceivedAt != nil {
		v.ReceivedAt = m.ReceivedAt.UTC().Format(time.RFC3339)
	}
	return v
}

func (a *officeMailTool) ListMyMessages(ctx context.Context, sessionID, accountID string, page, pageSize int) ([]mail.MessageView, int64, error) {
	userID, err := a.resolveUser(ctx, sessionID)
	if err != nil {
		return nil, 0, err
	}
	items, total, err := a.messages.List(ctx, userID, accountID, "", page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	out := make([]mail.MessageView, 0, len(items))
	for _, m := range items {
		out = append(out, mailMessageToView(m))
	}
	return out, total, nil
}

func (a *officeMailTool) ReadMyMessage(ctx context.Context, sessionID, messageID string) (*mail.MessageDetailView, error) {
	userID, err := a.resolveUser(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	m, err := a.messages.Get(ctx, messageID, userID)
	if err != nil {
		return nil, err
	}
	return &mail.MessageDetailView{MessageView: mailMessageToView(m), Body: m.Body}, nil
}

func (a *officeMailTool) SearchMyMessages(ctx context.Context, sessionID, query string, limit int) ([]mail.MessageView, error) {
	userID, err := a.resolveUser(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	items, err := a.messages.Search(ctx, userID, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]mail.MessageView, 0, len(items))
	for _, m := range items {
		out = append(out, mailMessageToView(m))
	}
	return out, nil
}

func (a *officeMailTool) ToTask(ctx context.Context, sessionID, messageID, titleOverride string) (string, error) {
	userID, err := a.resolveUser(ctx, sessionID)
	if err != nil {
		return "", err
	}
	return a.messages.MailToTask(ctx, messageID, userID, titleOverride)
}
