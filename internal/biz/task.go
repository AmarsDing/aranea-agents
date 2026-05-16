package biz

import (
	"context"
	"fmt"
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

type TaskRepo interface {
	SaveTask(ctx context.Context, task *GraphTask) error
	GetTask(ctx context.Context, taskID string) (*GraphTask, error)
	ListTasksByExecution(ctx context.Context, executionID string, status TaskStatus, pageSize int, pageToken string) ([]*GraphTask, string, error)
	UpdateTask(ctx context.Context, task *GraphTask) error

	SaveTaskComment(ctx context.Context, comment *TaskComment) error
	ListTaskComments(ctx context.Context, taskID string) ([]*TaskComment, error)

	SaveTaskLog(ctx context.Context, log *TaskLog) error
	ListTaskLogs(ctx context.Context, taskID string, stream string, level string, pageSize int) ([]*TaskLog, error)

	SaveTaskRun(ctx context.Context, run *TaskRun) error
	ListTaskRuns(ctx context.Context, taskID string) ([]*TaskRun, error)

	SaveTaskEvent(ctx context.Context, event *TaskEvent) error
	ListTaskEvents(ctx context.Context, executionID string, taskID string, eventType string, pageSize int) ([]*TaskEvent, error)
}

type AgentRoleChecker func(agentKey string, role string) bool
type AgentListerByRole func(role string) ([]string, error)

type TaskUsecase struct {
	repo          TaskRepo
	graphUC       *GraphUsecase
	roleChecker   AgentRoleChecker
	agentLister   AgentListerByRole
	mu            sync.RWMutex
	heartbeats    map[string]time.Time
	leaseDeadline map[string]time.Time
}

func NewTaskUsecase(repo TaskRepo, graphUC *GraphUsecase, agents AgentRepository) *TaskUsecase {
	return &TaskUsecase{
		repo:          repo,
		graphUC:       graphUC,
		roleChecker:   ProvideAgentRoleChecker(agents),
		agentLister:   ProvideAgentListerByRole(agents),
		heartbeats:    make(map[string]time.Time),
		leaseDeadline: make(map[string]time.Time),
	}
}

func (uc *TaskUsecase) CreateTask(ctx context.Context, nodeID string, executionID string, requiredRole string, assignmentMode string, assignmentStrategy string, input string, contextStr string) (*GraphTask, error) {
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
		agents, err := uc.agentLister(requiredRole)
		if err != nil || len(agents) == 0 {
			task.Status = TaskStatusPendingAssignment
		}
	}

	if err := uc.repo.SaveTask(ctx, task); err != nil {
		return nil, kerrors.InternalServer("TASK", fmt.Sprintf("task usecase create: %s", err.Error()))
	}

	uc.recordTaskEvent(ctx, task.TaskID, "task_created", nodeID, "task created")
	return task, nil
}

func (uc *TaskUsecase) GetTask(ctx context.Context, taskID string) (*GraphTask, error) {
	return uc.repo.GetTask(ctx, taskID)
}

func (uc *TaskUsecase) ListTasks(ctx context.Context, executionID string, status TaskStatus, pageSize int, pageToken string) ([]*GraphTask, string, error) {
	return uc.repo.ListTasksByExecution(ctx, executionID, status, pageSize, pageToken)
}

func (uc *TaskUsecase) ClaimTask(ctx context.Context, taskID string, agentKey string) (*GraphTask, error) {
	task, err := uc.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status != TaskStatusPending && task.Status != TaskStatusPendingAssignment {
		return nil, kerrors.BadRequest("TASK", fmt.Sprintf("task %s cannot be claimed in status %s", taskID, task.Status))
	}
	if task.AssignmentMode == "dynamic" && task.RequiredRole != "" {
		if uc.roleChecker != nil && !uc.roleChecker(agentKey, task.RequiredRole) {
			return nil, kerrors.Forbidden("TASK", fmt.Sprintf("agent %s does not have required role %s", agentKey, task.RequiredRole))
		}
	}
	now := time.Now()
	task.Assignee = agentKey
	task.Status = TaskStatusClaimed
	task.ClaimedAt = &now
	if err := uc.repo.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	uc.mu.Lock()
	uc.heartbeats[taskID] = now
	uc.leaseDeadline[taskID] = now.Add(5 * time.Minute)
	uc.mu.Unlock()
	uc.recordTaskEvent(ctx, taskID, "task_claimed", task.NodeID, "claimed by "+agentKey)
	return task, nil
}

func (uc *TaskUsecase) SubmitTaskResult(ctx context.Context, taskID string, output string, summary string, metadata string) (*GraphTask, error) {
	task, err := uc.repo.GetTask(ctx, taskID)
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
	if err := uc.repo.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	uc.recordTaskEvent(ctx, taskID, "task_completed", task.NodeID, "task result submitted")
	return task, nil
}

func (uc *TaskUsecase) Heartbeat(ctx context.Context, taskID string, agentKey string, metadata string) (bool, int32, error) {
	task, err := uc.repo.GetTask(ctx, taskID)
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
	task, err := uc.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status != TaskStatusClaimed {
		return nil, kerrors.BadRequest("TASK", fmt.Sprintf("task %s cannot be blocked in status %s", taskID, task.Status))
	}
	task.Status = TaskStatusBlocked
	task.Metadata = metadata
	if err := uc.repo.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	uc.recordTaskEvent(ctx, taskID, "task_blocked", task.NodeID, reason)
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
	if err := uc.repo.SaveTaskComment(ctx, comment); err != nil {
		return nil, err
	}
	return comment, nil
}

func (uc *TaskUsecase) ListTaskComments(ctx context.Context, taskID string) ([]*TaskComment, error) {
	return uc.repo.ListTaskComments(ctx, taskID)
}

func (uc *TaskUsecase) ListTaskLogs(ctx context.Context, taskID string, stream string, level string, pageSize int) ([]*TaskLog, error) {
	return uc.repo.ListTaskLogs(ctx, taskID, stream, level, pageSize)
}

func (uc *TaskUsecase) ListTaskRuns(ctx context.Context, taskID string) ([]*TaskRun, error) {
	return uc.repo.ListTaskRuns(ctx, taskID)
}

func (uc *TaskUsecase) ListTaskEvents(ctx context.Context, executionID string, taskID string, eventType string, pageSize int) ([]*TaskEvent, error) {
	return uc.repo.ListTaskEvents(ctx, executionID, taskID, eventType, pageSize)
}

func (uc *TaskUsecase) ReviewTask(ctx context.Context, taskID string, reviewerAgent string, approved bool, comment string) (*GraphTask, error) {
	task, err := uc.repo.GetTask(ctx, taskID)
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
	if err := uc.repo.UpdateTask(ctx, task); err != nil {
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
	return task, nil
}

func (uc *TaskUsecase) CheckTimeouts(ctx context.Context) error {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	now := time.Now()
	for taskID, deadline := range uc.leaseDeadline {
		if now.After(deadline) {
			lastHB, hasHB := uc.heartbeats[taskID]
			if hasHB && now.Sub(lastHB) < 2*time.Minute {
				uc.leaseDeadline[taskID] = now.Add(5 * time.Minute)
				continue
			}
			task, err := uc.repo.GetTask(ctx, taskID)
			if err != nil {
				continue
			}
			if task.Status == TaskStatusClaimed {
				task.Status = TaskStatusTimedOut
				_ = uc.repo.UpdateTask(ctx, task)
				uc.recordTaskEvent(ctx, taskID, "task_timed_out", task.NodeID, "task timed out")
			}
		}
	}
	return nil
}

func (uc *TaskUsecase) findNodeDef(ctx context.Context, task *GraphTask) *NodeDefInfo {
	exec, err := uc.graphUC.GetExecution(ctx, task.ExecutionID)
	if err != nil {
		return nil
	}
	def, err := uc.graphUC.GetGraph(ctx, exec.GraphID)
	if err != nil {
		return nil
	}
	cfg := defToBuildConfig(def)
	return uc.graphUC.factory.FindNodeDef(cfg, task.NodeID)
}

func (uc *TaskUsecase) recordTaskEvent(ctx context.Context, taskID string, eventType string, sourceNode string, description string) {
	event := &TaskEvent{
		EventID:     uuid.New().String(),
		TaskID:      taskID,
		EventType:   eventType,
		SourceNode:  sourceNode,
		Description: description,
		Timestamp:   time.Now(),
	}
	_ = uc.repo.SaveTaskEvent(ctx, event)
}
