package office

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"

	"github.com/google/uuid"
)

// 个人任务读写（工作台「我的任务」「交办中」与随手记录）。任务与邮件转
// 任务同表同仓储，属主隔离在 SQL 内完成（id + user_id）。
//
// 来源（source）决定一行任务出现在哪里：
//
//	manual / mail  我的任务（负责人是本人）
//	delegate       交办中（负责人是 Agent 或团队，source_ref = 承载会话 id）
//	note           随手记录

const (
	OfficeTaskOpen = "open"
	OfficeTaskDone = "done"

	OfficeTaskSourceManual   = "manual"
	OfficeTaskSourceMail     = "mail"
	OfficeTaskSourceDelegate = "delegate"
	OfficeTaskSourceNote     = "note"

	OfficeAssigneeAgent = "agent"
	OfficeAssigneeTeam  = "team"

	officeTaskTitleMax     = 200
	officeTaskBodyMax      = 8000
	officeTaskNameMax      = 80
	officeTaskDefaultLimit = 50
	officeTaskMaxLimit     = 200
)

var (
	ErrOfficeTaskNotFound       = apierror.NotFound(DomainOffice, "task not found")
	ErrOfficeTaskTitleRequired  = apierror.BadRequest(DomainOffice, "task title is required")
	ErrOfficeTaskStatusInvalid  = apierror.BadRequest(DomainOffice, "task status must be open or done")
	ErrOfficeTaskSourceInvalid  = apierror.BadRequest(DomainOffice, "task source must be manual, delegate or note")
	ErrOfficeTaskAssigneeNeeded = apierror.BadRequest(DomainOffice, "delegated task needs an agent or team assignee and its session")
)

// NewOfficeTaskInput 新建任务的入参。Source 为空按 manual。
type NewOfficeTaskInput struct {
	Title        string
	Body         string
	Source       string
	SourceRef    string
	AssigneeKind string
	AssigneeID   string
	AssigneeName string
}

func clipRunes(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}

// officeTaskListSources 把列表请求的来源翻成 SQL 过滤集：空 = 我的任务。
func officeTaskListSources(source string) ([]string, error) {
	switch s := strings.ToLower(strings.TrimSpace(source)); s {
	case "":
		return []string{OfficeTaskSourceManual, OfficeTaskSourceMail}, nil
	case OfficeTaskSourceManual, OfficeTaskSourceMail, OfficeTaskSourceDelegate, OfficeTaskSourceNote:
		return []string{s}, nil
	default:
		return nil, ErrOfficeTaskSourceInvalid
	}
}

func normalizeOfficeTaskStatus(status string) (string, error) {
	switch s := strings.ToLower(strings.TrimSpace(status)); s {
	case OfficeTaskOpen, OfficeTaskDone:
		return s, nil
	default:
		return "", ErrOfficeTaskStatusInvalid
	}
}

// ListTasks 返回本人任务；status 为空按 open，source 为空按「我的任务」
// （manual + mail），delegate / note 需显式指定。
func (u *MailMessageUsecase) ListTasks(ctx context.Context, userID, status, source string, limit int) ([]*OfficeTask, error) {
	sources, err := officeTaskListSources(source)
	if err != nil {
		return nil, err
	}
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
	return u.tasks.ListOfficeTasks(ctx, userID, st, sources, limit)
}

// CreateTask 新建一条本人名下的任务：手工任务、交办登记或随手记录。
// mail 来源只能由邮件转任务路径产生，这里不接受。
func (u *MailMessageUsecase) CreateTask(ctx context.Context, userID, workspaceID string, in NewOfficeTaskInput) (*OfficeTask, error) {
	title := clipRunes(strings.TrimSpace(in.Title), officeTaskTitleMax)
	if title == "" {
		return nil, ErrOfficeTaskTitleRequired
	}
	source := strings.ToLower(strings.TrimSpace(in.Source))
	if source == "" {
		source = OfficeTaskSourceManual
	}
	now := u.now()
	task := &OfficeTask{
		ID:          "otk_" + uuid.NewString(),
		WorkspaceID: workspaceID,
		UserID:      userID,
		Title:       title,
		Body:        clipRunes(strings.TrimSpace(in.Body), officeTaskBodyMax),
		Source:      source,
		Status:      OfficeTaskOpen,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	switch source {
	case OfficeTaskSourceManual, OfficeTaskSourceNote:
		// 负责人是本人，不记来源引用与交办对象。
	case OfficeTaskSourceDelegate:
		kind := strings.ToLower(strings.TrimSpace(in.AssigneeKind))
		task.SourceRef = strings.TrimSpace(in.SourceRef)
		task.AssigneeID = strings.TrimSpace(in.AssigneeID)
		if (kind != OfficeAssigneeAgent && kind != OfficeAssigneeTeam) || task.AssigneeID == "" || task.SourceRef == "" {
			return nil, ErrOfficeTaskAssigneeNeeded
		}
		task.AssigneeKind = kind
		task.AssigneeName = clipRunes(strings.TrimSpace(in.AssigneeName), officeTaskNameMax)
	default:
		return nil, ErrOfficeTaskSourceInvalid
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
