package biz

import (
	"aranea-agents/pkg/loggateway"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

type TaskStatus string

const (
	TaskStatusPending           TaskStatus = "pending"
	TaskStatusClaimed           TaskStatus = "claimed"
	TaskStatusComplete          TaskStatus = "complete"
	TaskStatusBlocked           TaskStatus = "blocked"
	TaskStatusReviewRequired    TaskStatus = "review_required"
	TaskStatusFailed            TaskStatus = "failed"
	TaskStatusTimedOut          TaskStatus = "timed_out"
	TaskStatusCancelled         TaskStatus = "cancelled"
	TaskStatusCrashed           TaskStatus = "crashed"
	TaskStatusPendingAssignment TaskStatus = "pending_assignment"
)

type GraphTask struct {
	TaskID             string
	NodeID             string
	ExecutionID        string
	Assignee           string
	Status             TaskStatus
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
	ListTasksByExecution(ctx context.Context, executionID string, status TaskStatus, pageSize int, pageToken string) ([]*GraphTask, string, error)
	ListTasksByStatuses(ctx context.Context, statuses []TaskStatus, limit int) ([]*GraphTask, error)
}

type TaskWriter interface {
	SaveTask(ctx context.Context, task *GraphTask) error
	UpdateTask(ctx context.Context, task *GraphTask) error
	BatchUpdateTaskStatus(ctx context.Context, taskIDs []string, status TaskStatus) error
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
	statusPublisher   TaskStatusPublisher
	completionHandler TaskCompletionHandler
	mu                sync.RWMutex
	heartbeats        map[string]time.Time
	leaseDeadline     map[string]time.Time
	lg                loggateway.Logger
}

func NewTaskUsecase(repo TaskRepo, graphUC TaskGraphResolver, agents AgentRepository, lg loggateway.Logger) *TaskUsecase {
	return &TaskUsecase{
		reader:      repo,
		writer:      repo,
		comments:    repo,
		logs:        repo,
		runs:        repo,
		events:      repo,
		graph:       graphUC,
		roleChecker: ProvideAgentRoleChecker(agents),
		agentLister: ProvideAgentListerByRole(agents),
		heartbeats:  make(map[string]time.Time),
		leaseDeadline: make(map[string]time.Time),
		lg:          lg,
	}
}

func (uc *TaskUsecase) SetLinkRepo(repo TaskLinkRepo) {
	uc.linkRepo = repo
}

func (uc *TaskUsecase) SetStatusPublisher(p TaskStatusPublisher) {
	uc.statusPublisher = p
}

func (uc *TaskUsecase) SetCompletionHandler(h TaskCompletionHandler) {
	uc.completionHandler = h
}

func (uc *TaskUsecase) afterTaskMutation(ctx context.Context, task *GraphTask, extra map[string]any) {
	if task == nil {
		return
	}
	uc.publishTaskStatus(ctx, task, extra)
	if task.Status != TaskStatusComplete {
		return
	}
	uc.promoteReadyChildren(ctx, task)
	if uc.completionHandler != nil {
		if err := uc.completionHandler.OnTaskCompleted(ctx, task); err != nil {
			uc.lg.Warn("OnTaskCompleted failed", loggateway.StepID("task.completion_handler"), loggateway.Err(err))
		}
	}
}

func (uc *TaskUsecase) CreateTask(ctx context.Context, nodeID string, executionID string, requiredRole string, assignmentMode string, assignmentStrategy string, input string, contextStr string) (*GraphTask, error) {
	if existing, err := uc.reader.GetActiveTaskByExecutionNode(ctx, executionID, nodeID); err == nil && existing != nil {
		return existing, nil
	}
	task := &GraphTask{
		TaskID:             uuid.New().String(),
		NodeID:             nodeID,
		ExecutionID:        executionID,
		Status:             TaskStatusPending,
		Input:              input,
		Context:            contextStr,
		RequiredRole:       requiredRole,
		AssignmentMode:     assignmentMode,
		AssignmentStrategy: assignmentStrategy,
		CreatedAt:          time.Now(),
	}

	if assignmentMode == "dynamic" && requiredRole != "" {
		agents, err := uc.agentLister(ctx, requiredRole)
		if err != nil || len(agents) == 0 {
			task.Status = TaskStatusPendingAssignment
		}
	}

	if err := uc.writer.SaveTask(ctx, task); err != nil {
		return nil, kerrors.InternalServer("TASK", fmt.Sprintf("task usecase create: %s", err.Error()))
	}

	uc.recordTaskEvent(ctx, task.TaskID, "task_created", nodeID, "task created")
	uc.afterTaskMutation(ctx, task, nil)
	return task, nil
}

func (uc *TaskUsecase) GetTask(ctx context.Context, taskID string) (*GraphTask, error) {
	return uc.reader.GetTask(ctx, taskID)
}

func (uc *TaskUsecase) ListTasks(ctx context.Context, executionID string, status TaskStatus, pageSize int, pageToken string) ([]*GraphTask, string, error) {
	return uc.reader.ListTasksByExecution(ctx, executionID, status, pageSize, pageToken)
}

func (uc *TaskUsecase) ListPendingTasks(ctx context.Context, limit int) ([]*GraphTask, error) {
	return uc.reader.ListTasksByStatuses(ctx, []TaskStatus{TaskStatusPending, TaskStatusPendingAssignment}, limit)
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
	if task.Status != TaskStatusPending && task.Status != TaskStatusPendingAssignment {
		return nil, kerrors.BadRequest("TASK", fmt.Sprintf("task %s cannot be claimed in status %s", taskID, task.Status))
	}
	if task.AssignmentMode == "dynamic" && task.RequiredRole != "" {
		if uc.roleChecker != nil && !uc.roleChecker(ctx, agentKey, task.RequiredRole) {
			return nil, kerrors.Forbidden("TASK", fmt.Sprintf("agent %s does not have required role %s", agentKey, task.RequiredRole))
		}
	}
	now := time.Now()
	task.Assignee = agentKey
	task.Status = TaskStatusClaimed
	task.ClaimedAt = &now
	if err := uc.writer.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	uc.mu.Lock()
	uc.heartbeats[taskID] = now
	uc.leaseDeadline[taskID] = now.Add(5 * time.Minute)
	uc.mu.Unlock()
	uc.recordTaskEvent(ctx, taskID, "task_claimed", task.NodeID, "claimed by "+agentKey)
	uc.afterTaskMutation(ctx, task, nil)
	return task, nil
}

func (uc *TaskUsecase) SubmitTaskResult(ctx context.Context, taskID string, output string, summary string, metadata string) (*GraphTask, error) {
	task, err := uc.reader.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status != TaskStatusClaimed && task.Status != TaskStatusReviewRequired {
		return nil, kerrors.BadRequest("TASK", fmt.Sprintf("task %s cannot submit result in status %s", taskID, task.Status))
	}
	now := time.Now()
	task.Output = output
	task.Summary = summary
	task.Metadata = metadata
	task.CompletedAt = &now

	nodeDef := uc.findNodeDef(ctx, task)
	if nodeDef != nil && nodeDef.ReviewerAgent != "" {
		task.Status = TaskStatusReviewRequired
	} else {
		task.Status = TaskStatusComplete
	}
	if err := uc.writer.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	uc.recordTaskEvent(ctx, taskID, "task_completed", task.NodeID, "task result submitted")
	uc.afterTaskMutation(ctx, task, nil)
	return task, nil
}

func (uc *TaskUsecase) Heartbeat(ctx context.Context, taskID string, agentKey string, metadata string) (bool, int32, error) {
	task, err := uc.reader.GetTask(ctx, taskID)
	if err != nil {
		return false, 0, err
	}
	if task.Status != TaskStatusClaimed {
		return false, 0, kerrors.BadRequest("TASK", fmt.Sprintf("task %s not in claimed status", taskID))
	}
	if task.Assignee != agentKey {
		return false, 0, kerrors.Forbidden("TASK", fmt.Sprintf("agent %s is not the assignee of task %s", agentKey, taskID))
	}
	now := time.Now()
	uc.mu.Lock()
	uc.heartbeats[taskID] = now
	var extension int32
	nodeDef := uc.findNodeDef(ctx, task)
	if nodeDef != nil && nodeDef.EnableLeaseExtension {
		newDeadline := now.Add(5 * time.Minute)
		uc.leaseDeadline[taskID] = newDeadline
		extension = 300
	}
	uc.mu.Unlock()
	uc.recordTaskEvent(ctx, taskID, "heartbeat", task.NodeID, "heartbeat received")
	return true, extension, nil
}

func (uc *TaskUsecase) ReportBlocked(ctx context.Context, taskID string, reason string, metadata string) (*GraphTask, error) {
	task, err := uc.reader.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status != TaskStatusClaimed {
		return nil, kerrors.BadRequest("TASK", fmt.Sprintf("task %s cannot be blocked in status %s", taskID, task.Status))
	}
	task.Status = TaskStatusBlocked
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
	if task.Status != TaskStatusBlocked {
		return nil, kerrors.BadRequest("TASK", fmt.Sprintf("task %s is not blocked", taskID))
	}
	task.Status = TaskStatusPending
	task.Assignee = ""
	task.ClaimedAt = nil
	if err := uc.writer.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	if comment != "" {
		_, _ = uc.AddTaskComment(ctx, taskID, "system", comment, "unblock")
	}
	uc.recordTaskEvent(ctx, taskID, "task_unblocked", task.NodeID, comment)
	uc.afterTaskMutation(ctx, task, nil)
	return task, nil
}

func (uc *TaskUsecase) CreateTaskWithParents(ctx context.Context, executionID, nodeID, requiredRole, assignmentMode, assignmentStrategy, input, contextStr string, parentIDs []string) (*GraphTask, error) {
	task, err := uc.CreateTask(ctx, nodeID, executionID, requiredRole, assignmentMode, assignmentStrategy, input, contextStr)
	if err != nil {
		return nil, err
	}
	linked := false
	for _, pid := range parentIDs {
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
		uc.publishTaskStatus(ctx, task, map[string]any{"parents_linked": true})
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
	if task.Status != TaskStatusReviewRequired {
		return nil, kerrors.BadRequest("TASK", fmt.Sprintf("task %s is not in review_required status", taskID))
	}
	if approved {
		task.Status = TaskStatusComplete
	} else {
		task.Status = TaskStatusClaimed
	}
	if err := uc.writer.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
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
		if task.Status == TaskStatusClaimed {
			timedOutIDs = append(timedOutIDs, task.TaskID)
			task.Status = TaskStatusTimedOut
			timedOutTasks = append(timedOutTasks, task)
		}
	}
	if len(timedOutIDs) == 0 {
		uc.lg.Info("task timeout check: no claimed tasks among expired",
			loggateway.StepID("task.timeout_check"),
			loggateway.Int("expired_count", len(expiredIDs)))
		return nil
	}
	if err := uc.writer.BatchUpdateTaskStatus(ctx, timedOutIDs, TaskStatusTimedOut); err != nil {
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
		uc.publishTaskStatus(ctx, task, nil)
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
	task.Status = TaskStatusPending
	task.Assignee = ""
	task.ClaimedAt = nil
	if err := uc.writer.UpdateTask(ctx, task); err != nil {
		uc.lg.Warn("release claim update failed",
			loggateway.StepID("task.release_claim_fail"),
			loggateway.Str("task_id", taskID), loggateway.Err(err))
	}
	uc.recordTaskEvent(ctx, taskID, "task_claim_released", task.NodeID, "claim released after dispatch failure")
	uc.publishTaskStatus(ctx, task, nil)
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
