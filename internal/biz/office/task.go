package office

import (
	"context"
	"net/http"
	"path"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"

	"github.com/google/uuid"
)

// 个人任务读写（工作台「我的任务」「进行中」与随手记录）。任务与邮件转
// 任务同表同仓储，属主隔离在 SQL 内完成（id + user_id）。
//
// 来源（source）只表示任务从哪来：
//
//	manual / mail  我建的 / 邮件转来的
//	delegate       指令栏一句话直接交办的（source_ref = 承载会话 id）
//	note           随手记录
//
// 谁在做看执行对象：assignee_kind 为空 = 本人；agent / team = 派给了 AI，
// session_id 是承载执行的会话。生命周期：open（待办）→ doing（我在做）→ done；
// 派给 AI 的任务保持 open，进度看承载会话（设计 2026-11-20-工作台任务生命周期.md §4）。

const (
	OfficeTaskOpen  = "open"
	OfficeTaskDoing = "doing"
	OfficeTaskDone  = "done"
	// officeTaskActive 列表专用：open + doing。
	officeTaskActive = "active"

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

	// OfficeTaskFileMaxBytes 单个附件上限（视频够用；BYTEA 入库，与邮件附件同款存法）。
	OfficeTaskFileMaxBytes = 50 << 20
	officeTaskFilesMax     = 20
	officeTaskFileNameMax  = 200
)

var (
	ErrOfficeTaskNotFound       = apierror.NotFound(DomainOffice, "task not found")
	ErrOfficeTaskTitleRequired  = apierror.BadRequest(DomainOffice, "task title is required")
	ErrOfficeTaskStatusInvalid  = apierror.BadRequest(DomainOffice, "task status must be open, doing or done")
	ErrOfficeTaskSourceInvalid  = apierror.BadRequest(DomainOffice, "task source must be manual, delegate or note")
	ErrOfficeTaskAssigneeNeeded = apierror.BadRequest(DomainOffice, "delegated task needs an agent or team assignee and its session")
	ErrOfficeTaskNoteAssign     = apierror.BadRequest(DomainOffice, "a note cannot be assigned")
	ErrOfficeTaskFileNotFound   = apierror.NotFound(DomainOffice, "task file not found")
	ErrOfficeTaskFileEmpty      = apierror.BadRequest(DomainOffice, "task file is empty")
	ErrOfficeTaskFileTooLarge   = apierror.BadRequest(DomainOffice, "task file exceeds 50MB")
	ErrOfficeTaskFilesFull      = apierror.BadRequest(DomainOffice, "a task can hold at most 20 files")
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
	DueAt        *time.Time
}

// UpdateOfficeTaskInput 改任务：nil 字段不动；ClearDue 清空到期时间（优先于 DueAt）。
type UpdateOfficeTaskInput struct {
	Title    *string
	Body     *string
	DueAt    *time.Time
	ClearDue bool
}

// AssignOfficeTaskInput 派给 AI：Kind 为空 = 收回自己做（其余字段忽略）。
type AssignOfficeTaskInput struct {
	Kind      string
	ID        string
	Name      string
	SessionID string
}

func clipRunes(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}

// officeTaskListFilter 把列表请求翻成仓储过滤：空 = 我的任务（我建的 + 邮件转来的，
// 且没派给 AI）；delegate = 派给了 AI 的（不论来源）；note = 随手记录。
func officeTaskListFilter(status, source string) (OfficeTaskFilter, error) {
	var f OfficeTaskFilter
	switch s := strings.ToLower(strings.TrimSpace(source)); s {
	case "":
		f.Sources, f.Assigned = []string{OfficeTaskSourceManual, OfficeTaskSourceMail}, -1
	case OfficeTaskSourceManual, OfficeTaskSourceMail:
		f.Sources, f.Assigned = []string{s}, -1
	case OfficeTaskSourceDelegate:
		f.Assigned = 1
	case OfficeTaskSourceNote:
		f.Sources = []string{s}
	default:
		return f, ErrOfficeTaskSourceInvalid
	}
	switch st := strings.ToLower(strings.TrimSpace(status)); st {
	case "", OfficeTaskOpen:
		f.Statuses = []string{OfficeTaskOpen}
	case officeTaskActive:
		f.Statuses = []string{OfficeTaskOpen, OfficeTaskDoing}
	case OfficeTaskDoing, OfficeTaskDone:
		f.Statuses = []string{st}
	default:
		return f, ErrOfficeTaskStatusInvalid
	}
	return f, nil
}

func normalizeOfficeTaskStatus(status string) (string, error) {
	switch s := strings.ToLower(strings.TrimSpace(status)); s {
	case OfficeTaskOpen, OfficeTaskDoing, OfficeTaskDone:
		return s, nil
	default:
		return "", ErrOfficeTaskStatusInvalid
	}
}

// ListTasks 返回本人任务；status 为空按 open（active = open + doing），source 为空按
// 「我的任务」，delegate / note 需显式指定。
func (u *MailMessageUsecase) ListTasks(ctx context.Context, userID, status, source string, limit int) ([]*OfficeTask, error) {
	filter, err := officeTaskListFilter(status, source)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = officeTaskDefaultLimit
	}
	if limit > officeTaskMaxLimit {
		limit = officeTaskMaxLimit
	}
	return u.tasks.ListOfficeTasks(ctx, userID, filter, limit)
}

// GetTask 读一条本人任务（他人的一律按不存在）。
func (u *MailMessageUsecase) GetTask(ctx context.Context, id, userID string) (*OfficeTask, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrOfficeTaskNotFound
	}
	t, err := u.tasks.GetOfficeTask(ctx, id, userID)
	if err != nil {
		return nil, ErrOfficeTaskNotFound
	}
	return t, nil
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
	case OfficeTaskSourceManual:
		task.DueAt = in.DueAt
	case OfficeTaskSourceNote:
		// 负责人是本人，不记来源引用、交办对象与到期。
	case OfficeTaskSourceDelegate:
		if err := applyAssignee(task, AssignOfficeTaskInput{
			Kind: in.AssigneeKind, ID: in.AssigneeID, Name: in.AssigneeName, SessionID: in.SourceRef,
		}); err != nil {
			return nil, err
		}
		task.SourceRef = task.SessionID
		task.DueAt = in.DueAt
	default:
		return nil, ErrOfficeTaskSourceInvalid
	}
	if err := u.tasks.CreateOfficeTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// applyAssignee 校验并写入执行对象与承载会话。
func applyAssignee(task *OfficeTask, in AssignOfficeTaskInput) error {
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	id, session := strings.TrimSpace(in.ID), strings.TrimSpace(in.SessionID)
	if (kind != OfficeAssigneeAgent && kind != OfficeAssigneeTeam) || id == "" || session == "" {
		return ErrOfficeTaskAssigneeNeeded
	}
	task.AssigneeKind, task.AssigneeID, task.SessionID = kind, id, session
	task.AssigneeName = clipRunes(strings.TrimSpace(in.Name), officeTaskNameMax)
	return nil
}

// SetTaskStatus 开始 / 完成 / 重开本人任务，返回更新后的任务。
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
	return u.GetTask(ctx, id, userID)
}

// UpdateTask 改标题、说明、到期时间。
func (u *MailMessageUsecase) UpdateTask(ctx context.Context, id, userID string, in UpdateOfficeTaskInput) (*OfficeTask, error) {
	task, err := u.GetTask(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if in.Title != nil {
		title := clipRunes(strings.TrimSpace(*in.Title), officeTaskTitleMax)
		if title == "" {
			return nil, ErrOfficeTaskTitleRequired
		}
		task.Title = title
	}
	if in.Body != nil {
		task.Body = clipRunes(strings.TrimSpace(*in.Body), officeTaskBodyMax)
	}
	switch {
	case in.ClearDue:
		task.DueAt = nil
	case in.DueAt != nil:
		task.DueAt = in.DueAt
	}
	task.UpdatedAt = u.now()
	if err := u.tasks.UpdateOfficeTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// AssignTask 派给 AI（记下执行对象与承载会话，任务回到 open 由会话推进），
// 或收回自己做（清掉执行对象，任务进 doing）。随手记录不能派。
func (u *MailMessageUsecase) AssignTask(ctx context.Context, id, userID string, in AssignOfficeTaskInput) (*OfficeTask, error) {
	task, err := u.GetTask(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if task.Source == OfficeTaskSourceNote {
		return nil, ErrOfficeTaskNoteAssign
	}
	if strings.TrimSpace(in.Kind) == "" {
		task.AssigneeKind, task.AssigneeID, task.AssigneeName, task.SessionID = "", "", "", ""
		task.Status = OfficeTaskDoing
		// 指令栏交办的任务没有别的来源；收回后它就是我自己的任务。
		if task.Source == OfficeTaskSourceDelegate {
			task.Source, task.SourceRef = OfficeTaskSourceManual, ""
		}
	} else {
		if err := applyAssignee(task, in); err != nil {
			return nil, err
		}
		task.Status = OfficeTaskOpen
	}
	task.UpdatedAt = u.now()
	if err := u.tasks.UpdateOfficeTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// DeleteTask 删除本人任务（附件级联；邮件上的转任务回链一并清掉）。
func (u *MailMessageUsecase) DeleteTask(ctx context.Context, id, userID string) error {
	if _, err := u.GetTask(ctx, id, userID); err != nil {
		return err
	}
	return u.tasks.DeleteOfficeTask(ctx, strings.TrimSpace(id), userID)
}

// ListTaskFiles 列任务附件（不含内容）。
func (u *MailMessageUsecase) ListTaskFiles(ctx context.Context, taskID, userID string) ([]*OfficeTaskFile, error) {
	if _, err := u.GetTask(ctx, taskID, userID); err != nil {
		return nil, err
	}
	return u.tasks.ListOfficeTaskFiles(ctx, strings.TrimSpace(taskID), userID)
}

// UploadTaskFile 给任务加一个附件。文件名只留最后一段；类型缺省按内容嗅探。
func (u *MailMessageUsecase) UploadTaskFile(ctx context.Context, taskID, userID, filename, contentType string, content []byte) (*OfficeTaskFile, error) {
	if len(content) == 0 {
		return nil, ErrOfficeTaskFileEmpty
	}
	if len(content) > OfficeTaskFileMaxBytes {
		return nil, ErrOfficeTaskFileTooLarge
	}
	existing, err := u.ListTaskFiles(ctx, taskID, userID)
	if err != nil {
		return nil, err
	}
	if len(existing) >= officeTaskFilesMax {
		return nil, ErrOfficeTaskFilesFull
	}
	name := path.Base(strings.ReplaceAll(strings.TrimSpace(filename), `\`, "/"))
	if name == "" || name == "." || name == "/" {
		name = "file"
	}
	ct := strings.TrimSpace(contentType)
	if ct == "" || ct == "application/octet-stream" {
		ct = http.DetectContentType(content)
	}
	f := &OfficeTaskFile{
		ID:          "otf_" + uuid.NewString(),
		TaskID:      strings.TrimSpace(taskID),
		UserID:      userID,
		Filename:    clipRunes(name, officeTaskFileNameMax),
		ContentType: clipRunes(ct, 200),
		Size:        int64(len(content)),
		Content:     content,
		CreatedAt:   u.now(),
	}
	if err := u.tasks.CreateOfficeTaskFile(ctx, f); err != nil {
		return nil, err
	}
	f.Content = nil
	return f, nil
}

// GetTaskFile 读附件（含内容），用于下载。
func (u *MailMessageUsecase) GetTaskFile(ctx context.Context, id, userID string) (*OfficeTaskFile, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrOfficeTaskFileNotFound
	}
	return u.tasks.GetOfficeTaskFile(ctx, id, userID)
}

// DeleteTaskFile 删附件。
func (u *MailMessageUsecase) DeleteTaskFile(ctx context.Context, id, userID string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrOfficeTaskFileNotFound
	}
	return u.tasks.DeleteOfficeTaskFile(ctx, id, userID)
}
