package biz

import (
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type GraphTaskStatus string

const (
	GraphTaskStatusPending           GraphTaskStatus = "pending"
	GraphTaskStatusClaimed           GraphTaskStatus = "claimed"
	GraphTaskStatusComplete          GraphTaskStatus = "complete"
	GraphTaskStatusBlocked           GraphTaskStatus = "blocked"
	GraphTaskStatusReviewRequired    GraphTaskStatus = "review_required"
	GraphTaskStatusFailed            GraphTaskStatus = "failed"
	GraphTaskStatusTimedOut          GraphTaskStatus = "timed_out"
	GraphTaskStatusCancelled         GraphTaskStatus = "cancelled"
	GraphTaskStatusCrashed           GraphTaskStatus = "crashed"
	GraphTaskStatusPendingAssignment GraphTaskStatus = "pending_assignment"
)

type GraphTask struct {
	TaskID             string
	NodeID             string
	ExecutionID        string
	Assignee           string
	Status             GraphTaskStatus
	Context            string
	Input              string
	Output             string
	Summary            string
	Metadata           string
	RequiredRole       string
	AssignmentMode     string
	AssignmentStrategy string
	CreatedAt          time.Time
	ClaimedAt          *time.Time
	CompletedAt        *time.Time
}

type TaskComment struct {
	CommentID string
	TaskID    string
	Author    string
	Content   string
	Type      string
	CreatedAt time.Time
}

type TaskLog struct {
	LogID     string
	TaskID    string
	Stream    string
	Content   string
	Level     string
	Timestamp time.Time
}

type TaskRun struct {
	RunID      string
	TaskID     string
	StartedAt  time.Time
	FinishedAt *time.Time
	ExitCode   int
	LogRef     string
}

type TaskEvent struct {
	EventID     string
	TaskID      string
	EventType   string
	SourceNode  string
	Description string
	Timestamp   time.Time
}

type TaskReader interface {
	GetTask(ctx context.Context, taskID string) (*GraphTask, error)
	GetTasksByIDs(ctx context.Context, taskIDs []string) ([]*GraphTask, error)
	GetActiveTaskByExecutionNode(ctx context.Context, executionID, nodeID string) (*GraphTask, error)
	ListTasksByExecution(ctx context.Context, executionID string, status GraphTaskStatus, pageSize int, pageToken string) ([]*GraphTask, string, error)
	ListTasksByStatuses(ctx context.Context, statuses []GraphTaskStatus, limit int) ([]*GraphTask, error)
}

type TaskWriter interface {
	SaveTask(ctx context.Context, task *GraphTask) error
	UpdateTask(ctx context.Context, task *GraphTask) error
	BatchUpdateGraphTaskStatus(ctx context.Context, taskIDs []string, status GraphTaskStatus) error
	// ClaimTaskWhereStatus 原子认领：仅当任务 status ∈ fromStatuses 时置为 claimed 并
	// 写入 assignee/claimed_at，返回最新任务。命中 0 行返回 (nil, false, nil)。
	// 消除 read-check-write 认领竞态（并发认领只允许一个成功）。
	ClaimTaskWhereStatus(ctx context.Context, taskID string, agentKey string, fromStatuses []GraphTaskStatus) (*GraphTask, bool, error)
	// CompleteTaskWhereStatus 原子提交：仅当 status ∈ fromStatuses 时写入结果字段并迁移到
	// toStatus；submitter 非空时附加 assignee=submitter 守卫（空串跳过，供无提交者上下文的
	// 调用方使用）。命中 0 行返回 (nil, false, nil)。
	CompleteTaskWhereStatus(ctx context.Context, taskID string, submitter string, output string, summary string, metadata string, toStatus GraphTaskStatus) (*GraphTask, bool, error)
	// TransitionTaskStatusWhere 原子状态迁移（不改其他字段），用于 Review 等纯流转场景。
	TransitionTaskStatusWhere(ctx context.Context, taskID string, fromStatuses []GraphTaskStatus, toStatus GraphTaskStatus) (*GraphTask, bool, error)
}

type TaskCommentStore interface {
	SaveTaskComment(ctx context.Context, comment *TaskComment) error
	ListTaskComments(ctx context.Context, taskID string) ([]*TaskComment, error)
}

type TaskLogStore interface {
	SaveTaskLog(ctx context.Context, log *TaskLog) error
	ListTaskLogs(ctx context.Context, taskID string, stream string, level string, pageSize int) ([]*TaskLog, error)
}

type TaskRunStore interface {
	SaveTaskRun(ctx context.Context, run *TaskRun) error
	ListTaskRuns(ctx context.Context, taskID string) ([]*TaskRun, error)
}

type TaskEventStore interface {
	SaveTaskEvent(ctx context.Context, event *TaskEvent) error
	ListTaskEvents(ctx context.Context, executionID string, taskID string, eventType string, pageSize int) ([]*TaskEvent, error)
}

type TaskRepo interface {
	TaskReader
	TaskWriter
	TaskCommentStore
	TaskLogStore
	TaskRunStore
	TaskEventStore
}

type AgentRoleChecker func(ctx context.Context, agentKey string, role string) bool
type AgentListerByRole func(ctx context.Context, role string) ([]string, error)

type TaskGraphResolver interface {
	GetExecution(ctx context.Context, executionID string) (*GraphExecution, error)
	FindGraphNode(ctx context.Context, graphID string, nodeID string) *NodeDef
	FindNodeDef(ctx context.Context, graphID string, nodeID string) *NodeTaskMeta
}

type TaskUsecase struct {
	reader            TaskReader
	writer            TaskWriter
	comments          TaskCommentStore
	logs              TaskLogStore
	runs              TaskRunStore
	events            TaskEventStore
	linkRepo          TaskLinkRepo
	graph             TaskGraphResolver
	roleChecker       AgentRoleChecker
	agentLister       AgentListerByRole
	statusPublisher   GraphTaskStatusPublisher
	completionHandler TaskCompletionHandler
	mu                sync.RWMutex
	heartbeats        map[string]time.Time
	leaseDeadline     map[string]time.Time
	lg                loggateway.Logger
}

func NewTaskUsecase(repo TaskRepo, graphUC TaskGraphResolver, agents AgentRepository, lg loggateway.Logger) *TaskUsecase {
	return &TaskUsecase{
		reader:        repo,
		writer:        repo,
		comments:      repo,
		logs:          repo,
		runs:          repo,
		events:        repo,
		graph:         graphUC,
		roleChecker:   ProvideAgentRoleChecker(agents),
		agentLister:   ProvideAgentListerByRole(agents),
		heartbeats:    make(map[string]time.Time),
		leaseDeadline: make(map[string]time.Time),
		lg:            lg,
	}
}

func (uc *TaskUsecase) SetLinkRepo(repo TaskLinkRepo) {
	uc.linkRepo = repo
}

func (uc *TaskUsecase) SetStatusPublisher(p GraphTaskStatusPublisher) {
	uc.statusPublisher = p
}

func (uc *TaskUsecase) SetCompletionHandler(h TaskCompletionHandler) {
	uc.completionHandler = h
}

func (uc *TaskUsecase) afterTaskMutation(ctx context.Context, task *GraphTask, extra map[string]any) {
	if task == nil {
		return
	}
	uc.publishGraphTaskStatus(ctx, task, extra)
	if task.Status != GraphTaskStatusComplete {
		return
	}
	uc.promoteReadyChildren(ctx, task)
	if uc.completionHandler != nil {
		if err := uc.completionHandler.OnTaskCompleted(ctx, task); err != nil {
			uc.lg.Warn("OnTaskCompleted failed", loggateway.StepID("task.completion_handler"), loggateway.Err(err))
		}
	}
}

// CreateTaskParams encapsulates parameters for creating a graph task.
// Introduced to keep CreateTask / CreateTaskWithParents signatures under the
// 5-parameter limit (S4/S5 fix). ParentIDs is optional and only honoured by
// CreateTaskWithParents.
type CreateTaskParams struct {
	NodeID             string
	ExecutionID        string
	RequiredRole       string
	AssignmentMode     string
	AssignmentStrategy string
	Input              string
	Context            string
	ParentIDs          []string
}

func (uc *TaskUsecase) CreateTask(ctx context.Context, p CreateTaskParams) (*GraphTask, error) {
	if existing, err := uc.reader.GetActiveTaskByExecutionNode(ctx, p.ExecutionID, p.NodeID); err == nil && existing != nil {
		return existing, nil
	}
	task := &GraphTask{
		TaskID:             uuid.New().String(),
		NodeID:             p.NodeID,
		ExecutionID:        p.ExecutionID,
		Status:             GraphTaskStatusPending,
		Input:              p.Input,
		Context:            p.Context,
		RequiredRole:       p.RequiredRole,
		AssignmentMode:     p.AssignmentMode,
		AssignmentStrategy: p.AssignmentStrategy,
		CreatedAt:          time.Now(),
	}

	if p.AssignmentMode == "dynamic" && p.RequiredRole != "" {
		agents, err := uc.agentLister(ctx, p.RequiredRole)
		if err != nil || len(agents) == 0 {
			task.Status = GraphTaskStatusPendingAssignment
		}
	}

	if err := uc.writer.SaveTask(ctx, task); err != nil {
		return nil, apierror.Internal("TASK", "task usecase create: %s", err)
	}

	uc.recordTaskEvent(ctx, task.TaskID, "task_created", p.NodeID, "task created")
	uc.afterTaskMutation(ctx, task, nil)
	return task, nil
}

func (uc *TaskUsecase) GetTask(ctx context.Context, taskID string) (*GraphTask, error) {
	return uc.reader.GetTask(ctx, taskID)
}

func (uc *TaskUsecase) ListTasks(ctx context.Context, executionID string, status GraphTaskStatus, pageSize int, pageToken string) ([]*GraphTask, string, error) {
	return uc.reader.ListTasksByExecution(ctx, executionID, status, pageSize, pageToken)
}

func (uc *TaskUsecase) ListPendingTasks(ctx context.Context, limit int) ([]*GraphTask, error) {
	return uc.reader.ListTasksByStatuses(ctx, []GraphTaskStatus{GraphTaskStatusPending, GraphTaskStatusPendingAssignment}, limit)
}

func (uc *TaskUsecase) SaveTaskRun(ctx context.Context, run *TaskRun) error {
	if run == nil {
		return nil
	}
	if run.RunID == "" {
		run.RunID = uuid.New().String()
	}
	return uc.runs.SaveTaskRun(ctx, run)
}

func (uc *TaskUsecase) ClaimTask(ctx context.Context, taskID string, agentKey string) (*GraphTask, error) {
	task, err := uc.reader.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status != GraphTaskStatusPending && task.Status != GraphTaskStatusPendingAssignment {
		return nil, apierror.BadRequest("TASK", "task %s cannot be claimed in status %s", taskID, task.Status)
	}
	if task.AssignmentMode == "dynamic" && task.RequiredRole != "" {
		if uc.roleChecker != nil && !uc.roleChecker(ctx, agentKey, task.RequiredRole) {
			return nil, apierror.Forbidden("TASK", "agent %s does not have required role %s", agentKey, task.RequiredRole)
		}
	}
	// 原子认领：消除并发认领竞态（此前 read-check-write 允许双认领，后写覆盖前写）。
	claimed, ok, err := uc.writer.ClaimTaskWhereStatus(ctx, taskID, agentKey, []GraphTaskStatus{GraphTaskStatusPending, GraphTaskStatusPendingAssignment})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apierror.Conflict("TASK", "task %s was concurrently claimed or changed status", taskID)
	}
	now := time.Now()
	uc.mu.Lock()
	uc.heartbeats[claimed.TaskID] = now
	uc.leaseDeadline[claimed.TaskID] = now.Add(5 * time.Minute)
	uc.mu.Unlock()
	uc.recordTaskEvent(ctx, claimed.TaskID, "task_claimed", claimed.NodeID, "claimed by "+agentKey)
	uc.afterTaskMutation(ctx, claimed, nil)
	return claimed, nil
}

// SubmitTaskResult 提交任务结果。submitter 为提交者 agentKey：非空时强制校验其为当前
// assignee（CAS 守卫），防止非认领者覆盖结果；为空时跳过 assignee 校验（服务端 RPC 暂无
// agent_key 字段，TODO(proto): SubmitTaskResultRequest 增加 agent_key 后接入）。
func (uc *TaskUsecase) SubmitTaskResult(ctx context.Context, taskID string, submitter string, output string, summary string, metadata string) (*GraphTask, error) {
	task, err := uc.reader.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status != GraphTaskStatusClaimed && task.Status != GraphTaskStatusReviewRequired {
		return nil, apierror.BadRequest("TASK", "task %s cannot submit result in status %s", taskID, task.Status)
	}
	if submitter != "" && task.Assignee != submitter {
		return nil, apierror.Forbidden("TASK", "agent %s is not the assignee of task %s", submitter, taskID)
	}

	nodeDef := uc.findNodeDef(ctx, task)
	toStatus := GraphTaskStatusComplete
	if nodeDef != nil && nodeDef.ReviewerAgent != "" {
		toStatus = GraphTaskStatusReviewRequired
	}
	// 原子提交：状态与 assignee 守卫在 UPDATE WHERE 中原子判定，防止过期租约/并发变更写入。
	updated, ok, err := uc.writer.CompleteTaskWhereStatus(ctx, taskID, submitter, output, summary, metadata, toStatus)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apierror.Conflict("TASK", "task %s state changed before submit (status/assignee mismatch)", taskID)
	}
	uc.recordTaskEvent(ctx, taskID, "task_completed", updated.NodeID, "task result submitted")
	uc.afterTaskMutation(ctx, updated, nil)
	return updated, nil
}

func (uc *TaskUsecase) Heartbeat(ctx context.Context, taskID string, agentKey string, metadata string) (bool, int32, error) {
	task, err := uc.reader.GetTask(ctx, taskID)
	if err != nil {
		return false, 0, err
	}
	if task.Status != GraphTaskStatusClaimed {
		return false, 0, apierror.BadRequest("TASK", "task %s not in claimed status", taskID)
	}
	if task.Assignee != agentKey {
		return false, 0, apierror.Forbidden("TASK", "agent %s is not the assignee of task %s", agentKey, taskID)
	}
	// 心跳即续约：无条件延长租约 5 分钟。租约超时的唯一目的是检测 agent
	// 死亡——心跳持续即 agent 存活，不应超时。原 EnableLeaseExtension 条件
	// 续约已被 CheckTimeouts 的心跳兜底（lastHB<2min 续 5min）架空，统一为
	// 无条件续约消除两者矛盾；同时 findNodeDef（DB 查询）不再于锁内执行。
	now := time.Now()
	uc.mu.Lock()
	uc.heartbeats[taskID] = now
	uc.leaseDeadline[taskID] = now.Add(5 * time.Minute)
	uc.mu.Unlock()
	uc.recordTaskEvent(ctx, taskID, "heartbeat", task.NodeID, "heartbeat received")
	return true, 300, nil
}

func (uc *TaskUsecase) ReportBlocked(ctx context.Context, taskID string, reason string, metadata string) (*GraphTask, error) {
	task, err := uc.reader.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status != GraphTaskStatusClaimed {
		return nil, apierror.BadRequest("TASK", "task %s cannot be blocked in status %s", taskID, task.Status)
	}
	task.Status = GraphTaskStatusBlocked
	task.Metadata = metadata
	if err := uc.writer.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	uc.recordTaskEvent(ctx, taskID, "task_blocked", task.NodeID, reason)
	uc.afterTaskMutation(ctx, task, nil)
	return task, nil
}

func (uc *TaskUsecase) UnblockTask(ctx context.Context, taskID string, comment string) (*GraphTask, error) {
	task, err := uc.reader.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status != GraphTaskStatusBlocked {
		return nil, apierror.BadRequest("TASK", "task %s is not blocked", taskID)
	}
	task.Status = GraphTaskStatusPending
	task.Assignee = ""
	task.ClaimedAt = nil
	if err := uc.writer.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	if comment != "" {
		if _, err := uc.AddTaskComment(ctx, taskID, "system", comment, "unblock"); err != nil {
			uc.lg.Warn("add task unblock comment failed", loggateway.Err(err), loggateway.Str("task_id", taskID))
		}
	}
	uc.recordTaskEvent(ctx, taskID, "task_unblocked", task.NodeID, comment)
	uc.afterTaskMutation(ctx, task, nil)
	return task, nil
}

func (uc *TaskUsecase) CreateTaskWithParents(ctx context.Context, p CreateTaskParams) (*GraphTask, error) {
	task, err := uc.CreateTask(ctx, p)
	if err != nil {
		return nil, err
	}
	linked := false
	for _, pid := range p.ParentIDs {
		pid = strings.TrimSpace(pid)
		if pid == "" {
			continue
		}
		if err := uc.LinkTasks(ctx, pid, task.TaskID); err != nil {
			return nil, err
		}
		linked = true
	}
	if linked {
		uc.publishGraphTaskStatus(ctx, task, map[string]any{"parents_linked": true})
	}
	return task, nil
}

func (uc *TaskUsecase) AddTaskComment(ctx context.Context, taskID string, author string, content string, commentType string) (*TaskComment, error) {
	comment := &TaskComment{
		CommentID: uuid.New().String(),
		TaskID:    taskID,
		Author:    author,
		Content:   content,
		Type:      commentType,
		CreatedAt: time.Now(),
	}
	if err := uc.comments.SaveTaskComment(ctx, comment); err != nil {
		return nil, err
	}
	return comment, nil
}

func (uc *TaskUsecase) ListTaskComments(ctx context.Context, taskID string) ([]*TaskComment, error) {
	return uc.comments.ListTaskComments(ctx, taskID)
}

func (uc *TaskUsecase) ListTaskLogs(ctx context.Context, taskID string, stream string, level string, pageSize int) ([]*TaskLog, error) {
	return uc.logs.ListTaskLogs(ctx, taskID, stream, level, pageSize)
}

func (uc *TaskUsecase) ListTaskRuns(ctx context.Context, taskID string) ([]*TaskRun, error) {
	return uc.runs.ListTaskRuns(ctx, taskID)
}

func (uc *TaskUsecase) ListTaskEvents(ctx context.Context, executionID string, taskID string, eventType string, pageSize int) ([]*TaskEvent, error) {
	return uc.events.ListTaskEvents(ctx, executionID, taskID, eventType, pageSize)
}

func (uc *TaskUsecase) ReviewTask(ctx context.Context, taskID string, reviewerAgent string, approved bool, comment string) (*GraphTask, error) {
	task, err := uc.reader.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status != GraphTaskStatusReviewRequired {
		return nil, apierror.BadRequest("TASK", "task %s is not in review_required status", taskID)
	}
	toStatus := GraphTaskStatusClaimed
	if approved {
		toStatus = GraphTaskStatusComplete
	}
	// 原子流转：防止 review 与超时清扫/并发提交竞态（非原子的 read-modify-write 会覆盖并发变更）。
	updated, ok, err := uc.writer.TransitionTaskStatusWhere(ctx, taskID, []GraphTaskStatus{GraphTaskStatusReviewRequired}, toStatus)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apierror.Conflict("TASK", "task %s left review_required before review completed", taskID)
	}
	task = updated
	commentType := "approval"
	if !approved {
		commentType = "rejection"
	}
	uc.AddTaskComment(ctx, taskID, reviewerAgent, comment, commentType)
	eventType := "task_review_approved"
	if !approved {
		eventType = "task_review_rejected"
	}
	uc.recordTaskEvent(ctx, taskID, eventType, task.NodeID, comment)
	uc.afterTaskMutation(ctx, task, map[string]any{"review_rejected": !approved, "review_comment": comment})
	return task, nil
}

func (uc *TaskUsecase) CheckTimeouts(ctx context.Context) error {
	type timeoutEntry struct {
		taskID   string
		deadline time.Time
	}
	var expired []timeoutEntry
	now := time.Now()
	uc.mu.Lock()
	for taskID, deadline := range uc.leaseDeadline {
		if now.After(deadline) {
			lastHB, hasHB := uc.heartbeats[taskID]
			if hasHB && now.Sub(lastHB) < 2*time.Minute {
				uc.leaseDeadline[taskID] = now.Add(5 * time.Minute)
				continue
			}
			expired = append(expired, timeoutEntry{taskID: taskID, deadline: deadline})
			delete(uc.leaseDeadline, taskID)
			delete(uc.heartbeats, taskID)
		}
	}
	uc.mu.Unlock()
	if len(expired) == 0 {
		return nil
	}
	uc.lg.Info("task timeout check: expired leases found",
		loggateway.StepID("task.timeout_check"),
		loggateway.Int("expired_count", len(expired)))
	expiredIDs := make([]string, 0, len(expired))
	for _, e := range expired {
		expiredIDs = append(expiredIDs, e.taskID)
	}
	tasks, err := uc.reader.GetTasksByIDs(ctx, expiredIDs)
	if err != nil {
		uc.lg.Error("task timeout check: batch get failed",
			loggateway.StepID("task.timeout_check"),
			loggateway.Int("expired_count", len(expiredIDs)), loggateway.Err(err))
		return err
	}
	var timedOutIDs []string
	var timedOutTasks []*GraphTask
	for _, task := range tasks {
		if task.Status == GraphTaskStatusClaimed {
			timedOutIDs = append(timedOutIDs, task.TaskID)
			task.Status = GraphTaskStatusTimedOut
			timedOutTasks = append(timedOutTasks, task)
		}
	}
	if len(timedOutIDs) == 0 {
		uc.lg.Info("task timeout check: no claimed tasks among expired",
			loggateway.StepID("task.timeout_check"),
			loggateway.Int("expired_count", len(expiredIDs)))
		return nil
	}
	if err := uc.writer.BatchUpdateGraphTaskStatus(ctx, timedOutIDs, GraphTaskStatusTimedOut); err != nil {
		uc.lg.Error("batch timeout update failed",
			loggateway.StepID("task.batch_timeout_fail"),
			loggateway.Int("count", len(timedOutIDs)), loggateway.Err(err))
		return err
	}
	uc.lg.Info("task timeout check: batch update succeeded",
		loggateway.StepID("task.timeout_check"),
		loggateway.Int("timed_out_count", len(timedOutIDs)))
	for _, task := range timedOutTasks {
		uc.recordTaskEvent(ctx, task.TaskID, "task_timed_out", task.NodeID, "task timed out")
		uc.publishGraphTaskStatus(ctx, task, nil)
	}
	return nil
}

func (uc *TaskUsecase) findNodeDef(ctx context.Context, task *GraphTask) *NodeTaskMeta {
	exec, err := uc.graph.GetExecution(ctx, task.ExecutionID)
	if err != nil {
		return nil
	}
	return uc.graph.FindNodeDef(ctx, exec.GraphID, task.NodeID)
}

func (uc *TaskUsecase) ReleaseClaim(ctx context.Context, taskID string) {
	uc.mu.Lock()
	delete(uc.leaseDeadline, taskID)
	delete(uc.heartbeats, taskID)
	uc.mu.Unlock()
	task, err := uc.reader.GetTask(ctx, taskID)
	if err != nil {
		return
	}
	task.Status = GraphTaskStatusPending
	task.Assignee = ""
	task.ClaimedAt = nil
	if err := uc.writer.UpdateTask(ctx, task); err != nil {
		uc.lg.Warn("release claim update failed",
			loggateway.StepID("task.release_claim_fail"),
			loggateway.Str("task_id", taskID), loggateway.Err(err))
	}
	uc.recordTaskEvent(ctx, taskID, "task_claim_released", task.NodeID, "claim released after dispatch failure")
	uc.publishGraphTaskStatus(ctx, task, nil)
}

func (uc *TaskUsecase) recordTaskEvent(ctx context.Context, taskID string, eventType string, sourceNode string, description string) {
	evt := &TaskEvent{
		EventID:     uuid.New().String(),
		TaskID:      taskID,
		EventType:   eventType,
		SourceNode:  sourceNode,
		Description: description,
		Timestamp:   time.Now(),
	}
	if err := uc.events.SaveTaskEvent(ctx, evt); err != nil {
		uc.lg.Warn("SaveTaskEvent failed", loggateway.StepID("task.event_save"), loggateway.Err(err))
	}
}

// === v2 Activity Ordering entities (Phase 1) ===
// The following types belong to the new LLM Activity Ordering model (spec §3.2.2),
// replacing the monolithic Activity struct. They are intentionally kept in task.go
// because the v1 GraphTask types above already occupy this file; the v2 Task
// entity uses the now-freed clean name (TaskStatus was renamed to GraphTaskStatus
// in v1 to free up this name).

// Task 是用户一次输入对应的根活动（v2 模型）。
// 替代旧 Activity 模型中 kind=task 的 root activity。
type Task struct {
	ID          string
	SessionID   string // = spirit_session_id
	UserMessage string
	Status      TaskStatus
	Seq         int64 // 在 session 内的序号
	Version     int64 // 乐观并发版本号（spec §3.3.5 VersionLT）
	// P2-B: tenant isolation. empty = legacy (treated as default workspace); non-empty = tenant-private.
	WorkspaceID string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
}

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
	// TaskStatusInterrupted marks a task terminalized by process restart while
	// in-flight (L3, 2026-07-22). Unlike failed, interrupted is resumable:
	// the user can explicitly continue execution via ResumeInterruptedTask,
	// which reruns the task with its full persisted execution trace.
	TaskStatusInterrupted TaskStatus = "interrupted"
)
