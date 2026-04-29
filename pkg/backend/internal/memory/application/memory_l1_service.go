// Package application 承载 `aranea/docs/13 memory-L1-working.md` 所述 L1 工作记忆的 MemoryL1Service。第一阶段提供
// 任务/字段 CRUD、提示渲染及 L2 归档所需快照。模式校验、TTL 到期与团队协调/工作者扇出在后续阶段（§11）复用同一面。
package application

import (
	mem "arenea/backend/internal/memory/domain"

	"arenea/backend/internal/kernel/errs"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// MemoryL1Service 为 ChatService/TeamRuntime/HTTP 使用的 L1 工作记忆门面。不自有状态；持久化委托 repository.Store，每次调用在内存中渲染提示，调用方总见最新字段值。
type MemoryL1Service struct {
	repo repository.Store
	now  func() string
}

// NewMemoryL1Service 构建 L1 服务。仓库须已执行规范 §3 迁移（memory_l1_* 表）。注入时钟以支持确定性测试。
func NewMemoryL1Service(repo repository.Store) *MemoryL1Service {
	return &MemoryL1Service{repo: repo, now: nowUTC}
}

// SetClock 覆盖时间源。测试用此固定时间戳。
func (s *MemoryL1Service) SetClock(now func() string) {
	if now != nil {
		s.now = now
	}
}

// StartL1TaskInput 为 StartTask 的参数对象。与域形状对应但可省略 ID/默认，使 ChatService 不必导入 repository。
type StartL1TaskInput struct {
	SessionID    string
	RunID        string
	TeamID       string
	AgentID      string
	TaskKey      string
	TaskTitle    string
	TaskGoal     string
	ParentTaskID string
	SchemaID     string
	BudgetTokens int
	SharedWith   []mem.L1FieldShare
	Metadata     map[string]any
}

// L1TaskView 将任务与当前字段及（可选）绑定的 JSON Schema 打包。HTTP 层直接渲染。
type L1TaskView struct {
	Task   mem.MemoryL1Task     `json:"task"`
	Fields []mem.MemoryL1Field  `json:"fields"`
	Schema *mem.MemoryL1Schema  `json:"schema,omitempty"`
}

// l1FieldPathPattern 实施规范 §5.2 第 2 步路径文法：
// `^[a-zA-Z_][a-zA-Z0-9_.]*$`，软长度上限 256 字符。
var l1FieldPathPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]{0,255}$`)

// StartTask 幂等：相同 (session, key, agent) 在未处于终态时返回已有行，不建重复。符合规范 §5.5「首条用户消息」挂钩，因 ChatService 可能因瞬态失败重试。
func (s *MemoryL1Service) StartTask(ctx context.Context, in StartL1TaskInput) (mem.MemoryL1Task, error) {
	if in.SessionID == "" {
		return mem.MemoryL1Task{}, validationError("session_id is required")
	}
	if in.TaskKey == "" {
		in.TaskKey = "default"
	}
	if existing, err := s.repo.GetL1TaskByKey(in.SessionID, in.TaskKey, in.AgentID); err == nil {
		if !existing.Status.IsTerminal() {
			return existing, nil
		}
	}

	budget := in.BudgetTokens
	if budget <= 0 {
		budget = s.resolveBudget(in.AgentID)
	}
	task := mem.MemoryL1Task{
		ID:           newID(),
		SessionID:    in.SessionID,
		RunID:        in.RunID,
		TeamID:       in.TeamID,
		AgentID:      in.AgentID,
		TaskKey:      in.TaskKey,
		TaskTitle:    in.TaskTitle,
		TaskGoal:     in.TaskGoal,
		Status:       mem.L1TaskActive,
		BudgetTokens: budget,
		ParentTaskID: in.ParentTaskID,
		SharedWith:   in.SharedWith,
		Metadata:     in.Metadata,
	}
	created, err := s.repo.CreateL1Task(task)
	if err != nil {
		return mem.MemoryL1Task{}, err
	}
	_ = s.audit("l1.start_task", "memory_l1_task", created.ID, map[string]any{
		"session": created.SessionID,
		"agent":   created.AgentID,
		"key":     created.TaskKey,
		"budget":  created.BudgetTokens,
	})

	if strings.TrimSpace(in.TaskGoal) != "" {
		if _, err = s.SetField(ctx, created.ID, mem.L1FieldPatch{
			FieldPath:    "task_goal",
			FieldKind:    "string",
			Value:        in.TaskGoal,
			Visibility:   "prompt",
			Source:       "system",
			ChangedBy:    "system",
			ChangeReason: "create",
		}); err != nil {
			return created, err
		}
	}
	return created, nil
}

// EndTask 将任务置为终态（completed/failed/…）并填写 ended_at。可重复调用：已终态则短路返回。
func (s *MemoryL1Service) EndTask(ctx context.Context, taskID string, status mem.L1TaskStatus) error {
	if taskID == "" {
		return validationError("task_id is required")
	}
	if !status.IsTerminal() {
		return validationError("end status must be terminal")
	}
	task, err := s.repo.GetL1TaskByID(taskID)
	if err != nil {
		return err
	}
	if task.Status.IsTerminal() {
		return nil
	}
	now := s.now()
	if err = s.repo.UpdateL1TaskStatus(taskID, status, now, ""); err != nil {
		return err
	}
	_ = s.audit("l1.end_task", "memory_l1_task", taskID, map[string]any{
		"status":      string(status),
		"used_tokens": task.UsedTokens,
		"agent":       task.AgentID,
		"session":     task.SessionID,
	})
	_ = ctx // 预留取消传播
	return nil
}

// GetTask 返回单任务的完整视图（任务+字段+可选 schema）。
func (s *MemoryL1Service) GetTask(ctx context.Context, taskID string) (L1TaskView, error) {
	if taskID == "" {
		return L1TaskView{}, validationError("task_id is required")
	}
	task, err := s.repo.GetL1TaskByID(taskID)
	if err != nil {
		return L1TaskView{}, err
	}
	fields, err := s.repo.ListL1FieldsByTask(taskID, true)
	if err != nil {
		return L1TaskView{}, err
	}
	view := L1TaskView{Task: task, Fields: fields}
	if schema := s.maybeLoadSchema(task.AgentID); schema != nil {
		view.Schema = schema
	}
	_ = ctx
	return view, nil
}

// GetTaskByKey 按 (session, key, agent) 解析任务。不存在则返回 ErrNotFound。
func (s *MemoryL1Service) GetTaskByKey(ctx context.Context, sessionID, taskKey, agentID string) (L1TaskView, error) {
	if sessionID == "" || taskKey == "" {
		return L1TaskView{}, validationError("session_id and task_key are required")
	}
	task, err := s.repo.GetL1TaskByKey(sessionID, taskKey, agentID)
	if err != nil {
		return L1TaskView{}, err
	}
	return s.GetTask(ctx, task.ID)
}

// ListActive 返回会话下所有非终态任务，最新在前。
func (s *MemoryL1Service) ListActive(ctx context.Context, sessionID string) ([]L1TaskView, error) {
	tasks, err := s.repo.ListL1TasksBySession(mem.L1TaskListQuery{SessionID: sessionID, IncludeEnded: false})
	if err != nil {
		return nil, err
	}
	out := make([]L1TaskView, 0, len(tasks))
	for _, task := range tasks {
		fields, ferr := s.repo.ListL1FieldsByTask(task.ID, true)
		if ferr != nil {
			return nil, ferr
		}
		out = append(out, L1TaskView{Task: task, Fields: fields})
	}
	_ = ctx
	return out, nil
}

// ListTasks 返回符合查询的任务。HTTP 层用于会话侧栏。
func (s *MemoryL1Service) ListTasks(ctx context.Context, query mem.L1TaskListQuery) ([]mem.MemoryL1Task, error) {
	_ = ctx
	return s.repo.ListL1TasksBySession(query)
}

// UpdateTaskShared 覆盖 shared_with 列表。团队协调者向工作者发布 `plan` 字段时使用。
func (s *MemoryL1Service) UpdateTaskShared(ctx context.Context, taskID string, shared []mem.L1FieldShare) error {
	_ = ctx
	if taskID == "" {
		return validationError("task_id is required")
	}
	return s.repo.UpdateL1TaskShared(taskID, shared)
}

// UpdateTaskBudget 修改每任务 token 预算。若新预算低于已用量则返回 422（会立即使任务溢出）。
func (s *MemoryL1Service) UpdateTaskBudget(ctx context.Context, taskID string, budgetTokens int) error {
	_ = ctx
	if taskID == "" {
		return validationError("task_id is required")
	}
	if budgetTokens <= 0 {
		return validationError("budget_tokens must be positive")
	}
	task, err := s.repo.GetL1TaskByID(taskID)
	if err != nil {
		return err
	}
	if task.UsedTokens > budgetTokens {
		return fmt.Errorf("%w: cannot lower budget below used tokens (%d > %d)", errs.ErrL1Overflow, task.UsedTokens, budgetTokens)
	}
	return s.repo.UpdateL1TaskBudget(taskID, budgetTokens)
}

// GetField 返回单字段。读计数惰性增加，便于 UI 展示「陈旧」字段以做裁剪决策（§9 到期）。
func (s *MemoryL1Service) GetField(ctx context.Context, taskID, fieldPath string) (mem.MemoryL1Field, error) {
	if taskID == "" || fieldPath == "" {
		return mem.MemoryL1Field{}, validationError("task_id and field_path are required")
	}
	field, err := s.repo.GetL1Field(taskID, fieldPath)
	if err != nil {
		return mem.MemoryL1Field{}, err
	}
	_ = s.repo.BumpL1FieldRead(field.ID, s.now())
	_ = ctx
	return field, nil
}

// ListFieldsByTask 列出任务字段。includeInternal 为 false 时过滤 visibility=internal（与 LLM 工具视图一致）。
func (s *MemoryL1Service) ListFieldsByTask(ctx context.Context, taskID string, includeInternal bool) ([]mem.MemoryL1Field, error) {
	_ = ctx
	if taskID == "" {
		return nil, validationError("task_id is required")
	}
	return s.repo.ListL1FieldsByTask(taskID, includeInternal)
}

// SetField 执行规范 §5.2 写流程：
//  1. 解析并锁定任务；拒绝终态
//  2. 校验字段路径/值
//  3. 强制执行每字段 token 上限
//  4. 应用乐观锁（IfRevision）
//  5. 除非 visibility=internal，强制执行每任务预算
//  6. 事务 upsert + 历史追加 + 预算重算
//  7. 写审计日志
func (s *MemoryL1Service) SetField(ctx context.Context, taskID string, patch mem.L1FieldPatch) (mem.MemoryL1Field, error) {
	_ = ctx
	if taskID == "" {
		return mem.MemoryL1Field{}, validationError("task_id is required")
	}
	if !l1FieldPathPattern.MatchString(patch.FieldPath) {
		return mem.MemoryL1Field{}, fmt.Errorf("%w: %q", errs.ErrInvalidFieldPath, patch.FieldPath)
	}
	task, err := s.repo.GetL1TaskByID(taskID)
	if err != nil {
		return mem.MemoryL1Field{}, err
	}
	if task.Status.IsTerminal() || task.Status == mem.L1TaskPaused {
		// paused 亦受写保护；仅 `active` 可写。
		if task.Status != mem.L1TaskActive {
			return mem.MemoryL1Field{}, fmt.Errorf("%w: status=%s", errs.ErrTaskNotWritable, task.Status)
		}
	}

	fieldKind := strings.TrimSpace(patch.FieldKind)
	if fieldKind == "" {
		fieldKind = "string"
	}
	visibility := strings.TrimSpace(patch.Visibility)
	if visibility == "" {
		visibility = "prompt"
	}
	pinToPrompt := true
	if patch.PinToPrompt != nil {
		pinToPrompt = *patch.PinToPrompt
	} else if visibility == "internal" {
		pinToPrompt = false
	}
	isRequired := false
	if patch.IsRequired != nil {
		isRequired = *patch.IsRequired
	}
	source := strings.TrimSpace(patch.Source)
	if source == "" {
		source = "agent"
	}
	changedBy := strings.TrimSpace(patch.ChangedBy)
	if changedBy == "" {
		changedBy = source
	}
	changeReason := strings.TrimSpace(patch.ChangeReason)
	if changeReason == "" {
		changeReason = "update"
	}

	valueText, valueJSON, err := encodeFieldValue(fieldKind, patch.Value)
	if err != nil {
		return mem.MemoryL1Field{}, fmt.Errorf("%w: %s", errs.ErrInvalidFieldValue, err.Error())
	}
	preview := strings.TrimSpace(patch.Preview)
	if preview == "" {
		preview = previewText(firstNonEmptyString(valueText, valueJSON, patch.ValueRef), l0PreviewLimit)
	}
	tokenEstimate := estimateFieldTokens(valueText, valueJSON, patch.ValueRef)

	settings := s.resolveSettings(task.AgentID)
	if tokenEstimate > settings.fieldMaxTokens {
		return mem.MemoryL1Field{}, fmt.Errorf("%w: %d > %d", errs.ErrFieldTooLarge, tokenEstimate, settings.fieldMaxTokens)
	}

	old, oldErr := s.repo.GetL1Field(taskID, patch.FieldPath)
	if oldErr != nil && !errors.Is(oldErr, errs.ErrNotFound) {
		return mem.MemoryL1Field{}, oldErr
	}
	exists := oldErr == nil

	if patch.IfRevision != nil {
		expected := *patch.IfRevision
		actual := 0
		if exists {
			actual = old.Revision
		}
		if expected != actual {
			return mem.MemoryL1Field{}, fmt.Errorf("%w: expected=%d actual=%d", errs.ErrRevisionConflict, expected, actual)
		}
	}

	revision := 1
	if exists {
		revision = old.Revision + 1
		if changeReason == "update" {
			changeReason = "patch"
		}
	} else if changeReason == "update" {
		changeReason = "create"
	}

	if visibility != "internal" {
		oldTokens := 0
		if exists && old.Visibility != "internal" {
			oldTokens = old.TokenEstimate
		}
		nextUsed := task.UsedTokens - oldTokens + tokenEstimate
		if nextUsed > task.BudgetTokens {
			return mem.MemoryL1Field{}, fmt.Errorf("%w: would-be %d > budget %d", errs.ErrL1Overflow, nextUsed, task.BudgetTokens)
		}
	}

	now := s.now()
	expiresAt := ""
	ttl := 0
	if patch.TTLSeconds != nil {
		ttl = *patch.TTLSeconds
		if ttl > 0 {
			expiresAt = time.Now().UTC().Add(time.Duration(ttl) * time.Second).Format(time.RFC3339)
		}
	}

	fieldID := ""
	if exists {
		fieldID = old.ID
	} else {
		fieldID = newID()
	}
	field := mem.MemoryL1Field{
		ID:            fieldID,
		TaskID:        taskID,
		SessionID:     task.SessionID,
		AgentID:       task.AgentID,
		FieldPath:     patch.FieldPath,
		FieldKind:     fieldKind,
		Visibility:    visibility,
		PinToPrompt:   pinToPrompt,
		IsRequired:    isRequired,
		ValueText:     valueText,
		ValueJSON:     valueJSON,
		ValueRef:      patch.ValueRef,
		Preview:       preview,
		TokenEstimate: tokenEstimate,
		Source:        source,
		SourceRef:     patch.SourceRef,
		TTLSeconds:    ttl,
		ExpiresAt:     expiresAt,
		Revision:      revision,
		Metadata:      patch.Metadata,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if exists {
		field.CreatedAt = old.CreatedAt
		field.LastReadAt = old.LastReadAt
		field.ReadCount = old.ReadCount
	}

	diff := buildFieldDiff(old, field, exists)
	history := mem.MemoryL1FieldHistory{
		ID:            newID(),
		FieldID:       field.ID,
		TaskID:        taskID,
		Revision:      revision,
		ValueText:     field.ValueText,
		ValueJSON:     field.ValueJSON,
		ValueRef:      field.ValueRef,
		Preview:       field.Preview,
		TokenEstimate: field.TokenEstimate,
		ChangedBy:     changedBy,
		ChangeReason:  changeReason,
		DiffJSON:      diff,
		MetadataJSON:  "{}",
		CreatedAt:     now,
	}

	stored, err := s.repo.UpsertL1Field(field, history, settings.keepRevisions)
	if err != nil {
		return mem.MemoryL1Field{}, err
	}
	_ = s.audit("l1.set_field", "memory_l1_field", stored.ID, map[string]any{
		"agent":    task.AgentID,
		"task":     taskID,
		"path":     patch.FieldPath,
		"reason":   changeReason,
		"old_rev":  oldRevisionFor(old, exists),
		"new_rev":  revision,
		"tokens":   tokenEstimate,
		"changed":  changedBy,
		"visible":  visibility,
	})
	return stored, nil
}

// PatchFields 按顺序执行多次 SetField。失败则中止余下补丁并返回首个错误。
//（第二阶段仓库提供批量 upsert 后可为单事务。）
func (s *MemoryL1Service) PatchFields(ctx context.Context, taskID string, patches []mem.L1FieldPatch) ([]mem.MemoryL1Field, error) {
	out := make([]mem.MemoryL1Field, 0, len(patches))
	for _, patch := range patches {
		stored, err := s.SetField(ctx, taskID, patch)
		if err != nil {
			return out, err
		}
		out = append(out, stored)
	}
	return out, nil
}

// DeleteField 从任务移除字段。历史行保留以便回滚。owner 任务的 used_tokens 在仓库事务内重算。
func (s *MemoryL1Service) DeleteField(ctx context.Context, taskID, fieldPath string) error {
	_ = ctx
	if taskID == "" || fieldPath == "" {
		return validationError("task_id and field_path are required")
	}
	field, err := s.repo.GetL1Field(taskID, fieldPath)
	if err != nil {
		return err
	}
	if err = s.repo.DeleteL1Field(field.ID); err != nil {
		return err
	}
	_ = s.audit("l1.delete_field", "memory_l1_field", field.ID, map[string]any{
		"task": taskID,
		"path": fieldPath,
	})
	return nil
}

// RollbackField 将旧版本复制回表头作为全新写入。历史中保留原版本号，审计轨迹仍完整。
func (s *MemoryL1Service) RollbackField(ctx context.Context, taskID, fieldPath string, toRevision int, changedBy string) (mem.MemoryL1Field, error) {
	_ = ctx
	if taskID == "" || fieldPath == "" {
		return mem.MemoryL1Field{}, validationError("task_id and field_path are required")
	}
	field, err := s.repo.GetL1Field(taskID, fieldPath)
	if err != nil {
		return mem.MemoryL1Field{}, err
	}
	hist, err := s.repo.GetL1FieldHistory(field.ID, toRevision)
	if err != nil {
		return mem.MemoryL1Field{}, err
	}
	patch := mem.L1FieldPatch{
		FieldPath:    field.FieldPath,
		FieldKind:    field.FieldKind,
		Visibility:   field.Visibility,
		PinToPrompt:  &field.PinToPrompt,
		IsRequired:   &field.IsRequired,
		Source:       field.Source,
		SourceRef:    field.SourceRef,
		ChangedBy:    firstNonEmptyString(changedBy, field.Source, "system"),
		ChangeReason: fmt.Sprintf("rollback:%d", toRevision),
		Value:        decodeHistoryValue(field.FieldKind, hist),
		ValueRef:     hist.ValueRef,
		Preview:      hist.Preview,
		Metadata:     field.Metadata,
	}
	return s.SetField(ctx, taskID, patch)
}

// ListFieldHistory 返回字段最近版本（最新在前）。
func (s *MemoryL1Service) ListFieldHistory(ctx context.Context, taskID, fieldPath string, limit int) ([]mem.MemoryL1FieldHistory, error) {
	_ = ctx
	if taskID == "" || fieldPath == "" {
		return nil, validationError("task_id and field_path are required")
	}
	field, err := s.repo.GetL1Field(taskID, fieldPath)
	if err != nil {
		return nil, err
	}
	return s.repo.ListL1FieldHistory(field.ID, limit)
}

// RenderForPrompt 构建 L0 注入系统消息的 markdown 块。可见性过滤同规范 §5.3：
//   - pin_to_prompt = false → 跳过
//   - visibility = internal → 跳过
//   - visibility = shared 且 viewer ∉ shared_with[field].read_by → 跳过
//   - expires_at < now → 跳过（未来 TTL 任务会标为 internal；此处不渲染值）
func (s *MemoryL1Service) RenderForPrompt(ctx context.Context, taskID, viewerAgentID string) (mem.L1PromptBlock, error) {
	_ = ctx
	if taskID == "" {
		return mem.L1PromptBlock{}, validationError("task_id is required")
	}
	task, err := s.repo.GetL1TaskByID(taskID)
	if err != nil {
		return mem.L1PromptBlock{}, err
	}
	fields, err := s.repo.ListL1FieldsByTask(taskID, false)
	if err != nil {
		return mem.L1PromptBlock{}, err
	}
	now := s.now()
	visible := make([]mem.MemoryL1Field, 0, len(fields))
	missing := []string{}
	for _, f := range fields {
		if !f.PinToPrompt {
			continue
		}
		if f.Visibility == "shared" && !taskShares(task, f.FieldPath, viewerAgentID) {
			continue
		}
		if f.ExpiresAt != "" && f.ExpiresAt < now {
			continue
		}
		if f.IsRequired && fieldHasValue(f) == false {
			missing = append(missing, f.FieldPath)
			continue
		}
		visible = append(visible, f)
	}
	body := renderFieldsAsMarkdown(task, visible)
	tokens := estimateTokensApprox(body)
	return mem.L1PromptBlock{
		Section:       "memory.l1",
		Role:          "system",
		Source:        "l1:" + task.ID,
		Tokens:        tokens,
		Content:       body,
		Preview:       previewText(body, l0PreviewLimit),
		MissingFields: missing,
		TaskID:        task.ID,
	}, nil
}

// RenderActiveTaskForPrompt 为 MemoryL0Service 入口。选取给定 (session, agent) 最近更新的活动任务并渲染。无合适任务时 ok=false，L0 可干净省略该片段。
func (s *MemoryL1Service) RenderActiveTaskForPrompt(ctx context.Context, sessionID, agentID string) (mem.L1PromptBlock, bool, error) {
	if sessionID == "" {
		return mem.L1PromptBlock{}, false, nil
	}
	tasks, err := s.repo.ListL1TasksBySession(mem.L1TaskListQuery{
		SessionID:    sessionID,
		AgentID:      agentID,
		Status:       string(mem.L1TaskActive),
		IncludeEnded: false,
	})
	if err != nil || len(tasks) == 0 {
		return mem.L1PromptBlock{}, false, err
	}
	task := tasks[0]
	block, err := s.RenderForPrompt(ctx, task.ID, agentID)
	if err != nil {
		return mem.L1PromptBlock{}, false, err
	}
	if strings.TrimSpace(block.Content) == "" && len(block.MissingFields) == 0 {
		return mem.L1PromptBlock{}, false, nil
	}
	return block, true, nil
}

// SnapshotForEpisode 产生 L2 情节管线（`aranea/docs/14`）消费的 JSON 视图。镜像所有非 internal 字段及任务标识与计数。
func (s *MemoryL1Service) SnapshotForEpisode(ctx context.Context, taskID string) (mem.L1Episode, error) {
	_ = ctx
	if taskID == "" {
		return mem.L1Episode{}, validationError("task_id is required")
	}
	task, err := s.repo.GetL1TaskByID(taskID)
	if err != nil {
		return mem.L1Episode{}, err
	}
	fields, err := s.repo.ListL1FieldsByTask(taskID, true)
	if err != nil {
		return mem.L1Episode{}, err
	}
	snapshot := make(map[string]any, len(fields))
	revisionCount := 0
	for _, f := range fields {
		snapshot[f.FieldPath] = map[string]any{
			"kind":        f.FieldKind,
			"visibility":  f.Visibility,
			"value":       fieldValueAsAny(f),
			"value_ref":   f.ValueRef,
			"revision":    f.Revision,
			"tokens":      f.TokenEstimate,
			"updated_at":  f.UpdatedAt,
			"source":      f.Source,
		}
		revisionCount += f.Revision
	}
	return mem.L1Episode{
		TaskID:       task.ID,
		SessionID:    task.SessionID,
		AgentID:      task.AgentID,
		TaskKey:      task.TaskKey,
		TaskTitle:    task.TaskTitle,
		TaskGoal:     task.TaskGoal,
		Status:       task.Status,
		StartedAt:    task.StartedAt,
		EndedAt:      task.EndedAt,
		UsedTokens:   task.UsedTokens,
		BudgetTokens: task.BudgetTokens,
		Snapshot:     snapshot,
		Stats: map[string]int{
			"fields":    len(fields),
			"revisions": revisionCount,
			"tokens":    task.UsedTokens,
		},
	}, nil
}

// ArchiveIdle 将 updated_at 早于 `before` 的活动任务置为 archived。返回数量供定时任务指标。
func (s *MemoryL1Service) ArchiveIdle(ctx context.Context, before string) (int, error) {
	_ = ctx
	if before == "" {
		return 0, validationError("before is required")
	}
	return s.repo.ArchiveIdleL1Tasks(before)
}

// UpsertSchema 存储或更新 schema 行。HTTP 层调用前会规范化 ID，同键重复 POST 返回 200 而非 409。
func (s *MemoryL1Service) UpsertSchema(ctx context.Context, schema mem.MemoryL1Schema) (mem.MemoryL1Schema, error) {
	_ = ctx
	if schema.SchemaKey == "" || schema.ScopeType == "" {
		return mem.MemoryL1Schema{}, validationError("scope_type and schema_key are required")
	}
	if schema.ID == "" {
		schema.ID = newID()
	}
	return s.repo.UpsertL1Schema(schema)
}

// ListSchemas 返回给定作用域元组下全部 schema。空过滤表示匹配全部，前端可列出所有。
func (s *MemoryL1Service) ListSchemas(ctx context.Context, scopeType, scopeID string) ([]mem.MemoryL1Schema, error) {
	_ = ctx
	return s.repo.ListL1Schemas(scopeType, scopeID)
}

// GetSchema 按 ID 返回 schema 行。
func (s *MemoryL1Service) GetSchema(ctx context.Context, id string) (mem.MemoryL1Schema, error) {
	_ = ctx
	if id == "" {
		return mem.MemoryL1Schema{}, validationError("id is required")
	}
	return s.repo.GetL1SchemaByID(id)
}

// DeleteSchema 删除 schema 行。通过 l1_default_schema_id 引用的任务仍可用——L0 渲染器仅省略「缺失必填字段」提示。
func (s *MemoryL1Service) DeleteSchema(ctx context.Context, id string) error {
	_ = ctx
	if id == "" {
		return validationError("id is required")
	}
	return s.repo.DeleteL1Schema(id)
}

// --- 辅助 --------------------------------------------------------------------

// l1Settings 为 L1 写路径解析的 agent_runtime_settings 子集。每次调用物化一次，使 SetField 主路径无分支。
type l1Settings struct {
	enabled        bool
	budget         int
	fieldMaxTokens int
	keepRevisions  int
	defaultSchema  string
	idleMinutes    int
}

func (s *MemoryL1Service) resolveSettings(agentID string) l1Settings {
	out := l1Settings{
		enabled:        true,
		budget:         8192,
		fieldMaxTokens: 2048,
		keepRevisions:  10,
		idleMinutes:    60,
	}
	if agentID == "" {
		return out
	}
	row, err := s.repo.GetAgentRuntimeSettings(agentID)
	if err != nil {
		return out
	}
	out.enabled = row.L1Enabled
	if row.L1BudgetTokens > 0 {
		out.budget = row.L1BudgetTokens
	}
	if row.L1FieldMaxTokens > 0 {
		out.fieldMaxTokens = row.L1FieldMaxTokens
	}
	if row.L1HistoryKeepRevisions > 0 {
		out.keepRevisions = row.L1HistoryKeepRevisions
	}
	if row.L1ArchiveOnIdleMinutes > 0 {
		out.idleMinutes = row.L1ArchiveOnIdleMinutes
	}
	out.defaultSchema = row.L1DefaultSchemaID
	return out
}

func (s *MemoryL1Service) resolveBudget(agentID string) int {
	return s.resolveSettings(agentID).budget
}

// maybeLoadSchema 若已配置则拉取智能体默认 schema。无 agent ID 或无法解析时返回 nil，GET 任务视图优雅降级。
func (s *MemoryL1Service) maybeLoadSchema(agentID string) *mem.MemoryL1Schema {
	settings := s.resolveSettings(agentID)
	if settings.defaultSchema == "" {
		return nil
	}
	schema, err := s.repo.GetL1SchemaByID(settings.defaultSchema)
	if err != nil {
		return nil
	}
	return &schema
}

func (s *MemoryL1Service) audit(action, resource, resourceID string, detail map[string]any) error {
	body, err := json.Marshal(detail)
	if err != nil {
		body = []byte("{}")
	}
	return s.repo.AddAuditLog(domain.AuditLog{
		ID:         newID(),
		Action:     action,
		Resource:   resource,
		ResourceID: resourceID,
		Detail:     string(body),
	})
}

// encodeFieldValue 将多态 patch.Value 转为落盘的 (text, json) 元组。列由 FieldKind 决定，下游无需自省。
func encodeFieldValue(kind string, value any) (string, string, error) {
	if value == nil {
		return "", "", nil
	}
	switch kind {
	case "string", "markdown", "reference":
		switch v := value.(type) {
		case string:
			return v, "", nil
		case fmt.Stringer:
			return v.String(), "", nil
		default:
			raw, err := json.Marshal(value)
			if err != nil {
				return "", "", err
			}
			return strings.Trim(string(raw), `"`), "", nil
		}
	case "number":
		raw, err := json.Marshal(value)
		if err != nil {
			return "", "", err
		}
		return string(raw), "", nil
	case "boolean":
		switch v := value.(type) {
		case bool:
			if v {
				return "true", "", nil
			}
			return "false", "", nil
		default:
			return "", "", fmt.Errorf("expected bool for kind=boolean, got %T", value)
		}
	case "json":
		raw, err := json.Marshal(value)
		if err != nil {
			return "", "", err
		}
		return "", string(raw), nil
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return "", "", err
		}
		return "", string(raw), nil
	}
}

func decodeHistoryValue(kind string, hist mem.MemoryL1FieldHistory) any {
	switch kind {
	case "json":
		var out any
		if hist.ValueJSON != "" {
			_ = json.Unmarshal([]byte(hist.ValueJSON), &out)
			return out
		}
	}
	if hist.ValueText != "" {
		return hist.ValueText
	}
	if hist.ValueJSON != "" {
		var out any
		_ = json.Unmarshal([]byte(hist.ValueJSON), &out)
		return out
	}
	return nil
}

func estimateFieldTokens(text, jsonValue, ref string) int {
	if text != "" {
		return estimateTokensApprox(text)
	}
	if jsonValue != "" {
		return estimateTokensApprox(jsonValue)
	}
	if ref != "" {
		// 引用通常 <100 字符；固定计 8 token，使其仍占预算。
		return 8
	}
	return 0
}

// taskShares 判断 viewer 是否可读任务的 path 字段。字段级授权优先于默认可见性。
func taskShares(task mem.MemoryL1Task, fieldPath, viewer string) bool {
	if viewer == "" || viewer == task.AgentID {
		return true
	}
	for _, share := range task.SharedWith {
		if share.Field != fieldPath && share.Field != "*" {
			continue
		}
		for _, agent := range share.ReadBy {
			if agent == viewer || agent == "team:*" {
				return true
			}
		}
	}
	return false
}

func fieldHasValue(f mem.MemoryL1Field) bool {
	return strings.TrimSpace(f.ValueText) != "" || strings.TrimSpace(f.ValueJSON) != "" || strings.TrimSpace(f.ValueRef) != ""
}

func fieldValueAsAny(f mem.MemoryL1Field) any {
	switch f.FieldKind {
	case "json":
		var out any
		if f.ValueJSON != "" {
			_ = json.Unmarshal([]byte(f.ValueJSON), &out)
			return out
		}
	case "boolean":
		return strings.EqualFold(f.ValueText, "true")
	case "number":
		var out any
		if f.ValueText != "" {
			_ = json.Unmarshal([]byte(f.ValueText), &out)
			return out
		}
	}
	if f.ValueText != "" {
		return f.ValueText
	}
	if f.ValueJSON != "" {
		var out any
		_ = json.Unmarshal([]byte(f.ValueJSON), &out)
		return out
	}
	if f.ValueRef != "" {
		return map[string]string{"$ref": f.ValueRef}
	}
	return nil
}

// renderFieldsAsMarkdown 与规范 §5.3 版式一致，模型见固定标题与列表。按 path 排序以保证提示确定性。
func renderFieldsAsMarkdown(task mem.MemoryL1Task, fields []mem.MemoryL1Field) string {
	if len(fields) == 0 {
		return ""
	}
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].FieldPath < fields[j].FieldPath })
	var b strings.Builder
	title := strings.TrimSpace(task.TaskTitle)
	if title == "" {
		title = strings.TrimSpace(task.TaskGoal)
	}
	if title == "" {
		title = task.TaskKey
	}
	fmt.Fprintf(&b, "## Working Memory（任务：%s）\n", title)
	for _, f := range fields {
		body := renderFieldBody(f)
		if body == "" {
			continue
		}
		fmt.Fprintf(&b, "- **%s**: %s\n", f.FieldPath, body)
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderFieldBody(f mem.MemoryL1Field) string {
	switch f.FieldKind {
	case "json":
		if f.ValueJSON == "" {
			return f.Preview
		}
		return f.ValueJSON
	case "reference":
		if f.ValueRef != "" {
			return f.ValueRef
		}
	}
	if f.ValueText != "" {
		return f.ValueText
	}
	if f.ValueRef != "" {
		return f.ValueRef
	}
	if f.ValueJSON != "" {
		return f.ValueJSON
	}
	return f.Preview
}

func buildFieldDiff(old mem.MemoryL1Field, next mem.MemoryL1Field, exists bool) string {
	if !exists {
		raw, _ := json.Marshal(map[string]any{
			"op":    "create",
			"value": fieldValueAsAny(next),
		})
		return string(raw)
	}
	raw, _ := json.Marshal(map[string]any{
		"op":   "update",
		"from": fieldValueAsAny(old),
		"to":   fieldValueAsAny(next),
	})
	return string(raw)
}

func oldRevisionFor(old mem.MemoryL1Field, exists bool) int {
	if !exists {
		return 0
	}
	return old.Revision
}
