package office

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"

	"github.com/google/uuid"
)

// 个人任务读写（工作台「我的任务」）。任务与邮件转任务同表同仓储，
// 属主隔离在 SQL 内完成（id + user_id）。

const (
	OfficeTaskOpen = "open"
	OfficeTaskDone = "done"

	officeTaskTitleMax     = 200
	officeTaskDefaultLimit = 50
	officeTaskMaxLimit     = 200
)

var (
	ErrOfficeTaskNotFound      = apierror.NotFound(DomainOffice, "task not found")
	ErrOfficeTaskTitleRequired = apierror.BadRequest(DomainOffice, "task title is required")
	ErrOfficeTaskStatusInvalid = apierror.BadRequest(DomainOffice, "task status must be open or done")
)

func normalizeOfficeTaskStatus(status string) (string, error) {
	switch s := strings.ToLower(strings.TrimSpace(status)); s {
	case OfficeTaskOpen, OfficeTaskDone:
		return s, nil
	default:
		return "", ErrOfficeTaskStatusInvalid
	}
}

// ListTasks 返回本人任务；status 为空按 open。
func (u *MailMessageUsecase) ListTasks(ctx context.Context, userID, status string, limit int) ([]*OfficeTask, error) {
	if strings.TrimSpace(status) == "" {
		status = OfficeTaskOpen
	}
	st, err := normalizeOfficeTaskStatus(status)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = officeTaskDefaultLimit
	}
	if limit > officeTaskMaxLimit {
		limit = officeTaskMaxLimit
	}
	return u.tasks.ListOfficeTasks(ctx, userID, st, limit)
}

// CreateTask 手工新建一条本人任务。
func (u *MailMessageUsecase) CreateTask(ctx context.Context, userID, workspaceID, title, body string) (*OfficeTask, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, ErrOfficeTaskTitleRequired
	}
	if r := []rune(title); len(r) > officeTaskTitleMax {
		title = string(r[:officeTaskTitleMax])
	}
	now := u.now()
	task := &OfficeTask{
		ID:          "otk_" + uuid.NewString(),
		WorkspaceID: workspaceID,
		UserID:      userID,
		Title:       title,
		Body:        strings.TrimSpace(body),
		Source:      "manual",
		Status:      OfficeTaskOpen,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := u.tasks.CreateOfficeTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// SetTaskStatus 完成或重开本人任务，返回更新后的任务。
func (u *MailMessageUsecase) SetTaskStatus(ctx context.Context, id, userID, status string) (*OfficeTask, error) {
	st, err := normalizeOfficeTaskStatus(status)
	if err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrOfficeTaskNotFound
	}
	if err := u.tasks.SetOfficeTaskStatus(ctx, id, userID, st, u.now()); err != nil {
		return nil, err
	}
	task, err := u.tasks.GetOfficeTask(ctx, id, userID)
	if err != nil {
		return nil, ErrOfficeTaskNotFound
	}
	return task, nil
}
