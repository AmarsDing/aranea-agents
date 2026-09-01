package biz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// DeptLeadAgentKeyPrefix is the prefix for department lead agent keys.
// Full key pattern: __dept_lead_{dept_key}__
const DeptLeadAgentKeyPrefix = "__dept_lead_"

// maxCrossDeptRatio is the maximum allowed ratio of cross-department members
// to total enabled members in a team (50%).
const maxCrossDeptRatio = 0.5

// DeptLeadManagerOpts holds the dependencies for DeptLeadManager.
type DeptLeadManagerOpts struct {
	OrgRepo    OrganizationRepo
	BorrowRepo BorrowRequestRepo
	AgentRepo  AgentRepository
	AgentUC    *AgentUsecase
	TeamGetter DeptLeadTeamGetter
	EventBus   EventBus
	Logger     loggateway.Logger
}

// DeptLeadTeamGetter is a narrow interface for fetching team info needed by DeptLeadManager.
type DeptLeadTeamGetter interface {
	GetTeamByID(ctx context.Context, id string) (Team, error)
}

// DeptLeadManager manages department lead agents.
// Each department automatically gets a system_builtin Agent as its lead.
type DeptLeadManager struct {
	orgRepo    OrganizationRepo
	borrowRepo BorrowRequestRepo
	agentRepo  AgentRepository
	agentUC    *AgentUsecase
	teamGetter DeptLeadTeamGetter
	eventBus   EventBus
	lg         loggateway.Logger
}

func NewDeptLeadManager(opts DeptLeadManagerOpts) *DeptLeadManager {
	return &DeptLeadManager{
		orgRepo:    opts.OrgRepo,
		borrowRepo: opts.BorrowRepo,
		agentRepo:  opts.AgentRepo,
		agentUC:    opts.AgentUC,
		teamGetter: opts.TeamGetter,
		eventBus:   opts.EventBus,
		lg:         opts.Logger,
	}
}

// governanceLeadSettings returns the canonical runtime settings for a dept
// lead: read_only tool face (监管工具由身份注入，org-invariants §3) + 记忆栈
// 全开。Boolean fields are set explicitly because withSettingDefaults does
// not backfill zero-value bools, and Ent Create writes every field — a
// zero-value bool overrides the DB column default (2026-08-24 gov-lead
// audit F4)。company_lead 在 company_lead.go 有其 R10 变体
// （subagents/heartbeat 保持 false）。
// evo_* 保持零值 false：治理岗 prompt 由模板渲染，不进进化候选。
func governanceLeadSettings(agentID string) AgentRuntimeSettings {
	settings := withSettingDefaults(AgentRuntimeSettings{AgentID: agentID})
	settings.MemoryEnabled = true
	settings.ToolsEnabled = true
	settings.ToolsProfile = "read_only"
	settings.L0InjectL1 = true
	settings.L0InjectL3 = true
	settings.L0InjectL4 = true
	settings.L0SnapshotEnabled = true
	settings.L1Enabled = true
	settings.L2EpisodeEnabled = true
	settings.L2IndexEnabled = true
	settings.L2RecallEnabled = true
	settings.L3Enabled = true
	settings.L3InjectProvenance = true
	settings.L4Enabled = true
	settings.L4GraphInjectNeighbors = true
	settings.L4IdentityInject = true
	settings.SubagentsEnabled = true
	settings.IntentPassEnabled = true
	settings.ClarificationEnabled = true
	settings.IntentSkipEnabled = false
	return settings
}

// CreateDeptLead creates a department lead Agent for the given department.
// Called automatically when a new department is created.
func (m *DeptLeadManager) CreateDeptLead(ctx context.Context, deptNode OrganizationNode) (*Agent, error) {
	if deptNode.Level != "department" {
		return nil, apierror.BadRequest("DEPT_LEAD", "can only create lead for department level nodes")
	}

	agentKey := fmt.Sprintf("__dept_lead_%s__", deptNode.Key)

	// Check if dept lead already exists (idempotent)
	existing, err := m.agentRepo.GetAgentByAgentKey(ctx, agentKey)
	if err == nil && existing.ID != "" {
		// Already exists; ensure org node is linked
		if deptNode.DeptLeadAgentID != existing.ID {
			deptNode.DeptLeadAgentID = existing.ID
			if _, updateErr := m.orgRepo.UpdateOrgNode(ctx, deptNode); updateErr != nil {
				m.lg.Warn("failed to link existing dept lead to org node",
					loggateway.StepID("dept_lead.create"),
					loggateway.Str("dept_id", deptNode.ID),
					loggateway.Err(updateErr))
			}
		}
		a, _ := m.agentUC.Get(ctx, existing.ID)
		return &a, nil
	}

	agent := Agent{
		AgentKey:     agentKey,
		DisplayName:  fmt.Sprintf("部门主管-%s", deptNode.Name),
		Kind:         "system_builtin",
		Source:       "system",
		PositionID:   deptNode.ID,
		PositionKey:  deptNode.Key,
		AgentVariant: "dept_lead",
		Status:       "active",
		Readonly:     true,
	}

	if agent.ID == "" {
		agent.ID = newAgentCatalogID()
	}

	// Build system prompt for dept lead
	prompt := m.buildDeptLeadPrompt(deptNode)
	agent.Files = []AgentPromptFile{
		{ID: newAgentCatalogID(), AgentID: agent.ID, Name: "system.md", Body: prompt, SortOrder: 1},
	}

	// Default settings for dept lead agent
	settings := governanceLeadSettings(agent.ID)

	// Build config_json from settings
	configJSON, cfgErr := configJSONFromSettings(settings, agent.Files)
	if cfgErr != nil {
		return nil, apierror.Internal("DEPT_LEAD", "failed to build dept lead config: %s", cfgErr)
	}
	agent.ConfigJSON = EmbedAgentKindInConfigJSON(configJSON, AgentKindLLM, nil, m.lg)

	// Create agent atomically (agent + settings + files)
	created, err := m.agentUC.CreateWithFilesAndSettings(ctx, agent, agent.Files, &settings)
	if err != nil {
		return nil, err
	}

	// Update department node with lead agent ID
	deptNode.DeptLeadAgentID = created.ID
	_, err = m.orgRepo.UpdateOrgNode(ctx, deptNode)
	if err != nil {
		return nil, err
	}

	return &created, nil
}

// DeleteDeptLead deletes the department lead Agent.
// Called when a department is deleted.
func (m *DeptLeadManager) DeleteDeptLead(ctx context.Context, deptID string) error {
	node, err := m.orgRepo.GetOrgNode(ctx, deptID)
	if err != nil {
		return err
	}
	if node.DeptLeadAgentID == "" {
		return nil
	}
	return m.agentUC.ForceDelete(ctx, node.DeptLeadAgentID)
}

// ReplaceDeptLead replaces the department lead Agent by deleting the old one
// and creating a new one.
func (m *DeptLeadManager) ReplaceDeptLead(ctx context.Context, deptID string) (*Agent, error) {
	if err := m.DeleteDeptLead(ctx, deptID); err != nil {
		return nil, err
	}
	node, err := m.orgRepo.GetOrgNode(ctx, deptID)
	if err != nil {
		return nil, err
	}
	return m.CreateDeptLead(ctx, node)
}

// GetDeptLeadForTeam returns the department lead Agent for a team's department.
func (m *DeptLeadManager) GetDeptLeadForTeam(ctx context.Context, deptID string) (*Agent, error) {
	node, err := m.orgRepo.GetOrgNode(ctx, deptID)
	if err != nil {
		return nil, err
	}
	if node.DeptLeadAgentID == "" {
		return nil, apierror.NotFound("DEPT_LEAD", "department has no lead agent")
	}
	a, err := m.agentUC.Get(ctx, node.DeptLeadAgentID)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// GetDeptLeadAgent returns the dept lead agent by its agent ID.
func (m *DeptLeadManager) GetDeptLeadAgent(ctx context.Context, agentID string) (*Agent, error) {
	a, err := m.agentUC.Get(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// extractSystemPrompt extracts the system prompt from an agent's Files.
// Looks for a file named "system.md"; falls back to empty string if not found.
func extractSystemPrompt(agent *Agent) string {
	if agent == nil {
		return ""
	}
	for _, f := range agent.Files {
		if f.Name == "system.md" {
			return f.Body
		}
	}
	return ""
}

// deptLeadPromptData holds the template variables for dept_lead.md.
type deptLeadPromptData struct {
	DepartmentName        string
	DepartmentDescription string
}

// deptLeadPromptTmpl is the parsed template for dept_lead.md.
// Parsed once at init; if the file is missing, falls back to inline default.
var deptLeadPromptTmpl *template.Template

func init() {
	tmplPath := filepath.Join(ScenarioDir(), "system", "prompts", "dept_lead.md")
	tmplBytes, err := os.ReadFile(tmplPath)
	if err != nil {
		// Fallback: inline template matching dept_lead.md content
		deptLeadPromptTmpl = template.Must(template.New("dept_lead_fallback").Parse(`# 部门主管

你是「{{.DepartmentName}}」的部门主管。

## 职责

1. **资源协调**：管理本部门的人力资源分配，审批跨部门借调请求
2. **质量把关**：审核本部门产出的交付物质量
3. **验收确认**：确认其他部门交付给本部门的工作是否满足需求

## 审批规则

- 跨部门交付物需要双方主管确认（输出方质量把关 + 接收方验收确认）
- 借调成员加入其他 Team 时，你需要审批同意
- 你自动加入本部门的所有 Team
- 借调请求超过 5 分钟未处理，系统自动批准

## 跨部门沟通

你可以使用部门信箱（deptmail）工具与其他部门主管进行异步沟通：

- **send_dept_message**：向其他部门主管发送消息（需提供目标部门 ID、主题、正文）
- **list_inbox**：查看本人收件箱中的消息摘要
- **read_message**：读取消息完整内容（自动标记已读）
- **reply_message**：回复收到的消息，形成消息线程

适用场景：
- 借调前协商：在发起正式借调请求前，先与对方部门主管沟通借调意向和条件
- 交付物标准对齐：跨部门交付物开始前，与对方主管确认交付标准和验收条件
- 质量争议处理：交付物被退回或验收不通过时，与对方主管沟通修订方案
- 资源协调：本部门资源紧张时，与其他部门主管协商人力调配

注意：部门信箱是异步沟通工具，发送后对方主管会被自动唤醒查收。不能发送给本部门。

## 部门信息

- 部门名称：{{.DepartmentName}}
- 部门描述：{{.DepartmentDescription}}`))
		return
	}
	deptLeadPromptTmpl = template.Must(template.New("dept_lead").Parse(string(tmplBytes)))
}

// buildDeptLeadPrompt generates the system prompt for a department lead
// by loading and rendering the dept_lead.md template.
func (m *DeptLeadManager) buildDeptLeadPrompt(dept OrganizationNode) string {
	desc := strings.TrimSpace(dept.Description)
	if desc == "" {
		desc = "（无描述）"
	}
	data := deptLeadPromptData{
		DepartmentName:        dept.Name,
		DepartmentDescription: desc,
	}
	var buf strings.Builder
	if err := deptLeadPromptTmpl.Execute(&buf, data); err != nil {
		// Should never happen with a valid template and string data
		m.lg.Warn("failed to render dept lead prompt template, using fallback",
			loggateway.StepID("dept_lead.prompt"),
			loggateway.Err(err),
		)
		return fmt.Sprintf("你是「%s」的部门主管。\n\n部门描述：%s", dept.Name, desc)
	}
	return buf.String()
}

// ---------------------------------------------------------------------------
// Borrow Request (DL-09 / DL-10)
// ---------------------------------------------------------------------------

// BorrowRequestStatus constants for borrow request lifecycle.
const (
	BorrowRequestPending      = "pending"
	BorrowRequestApproved     = "approved"
	BorrowRequestRejected     = "rejected"
	BorrowRequestAutoApproved = "auto_approved"
)

// BorrowAutoApproveTimeout is the duration after which a pending borrow request
// is automatically approved.
const BorrowAutoApproveTimeout = 5 * time.Minute

// BorrowRequest represents a cross-department agent borrowing request.
// When a team wants to use an agent from another department, the lending
// department's lead must approve before the agent can join.
type BorrowRequest struct {
	ID           string
	TeamID       string
	AgentID      string
	FromDeptID   string // department that owns the agent
	ToDeptID     string // department that wants to borrow the agent
	Status       string // pending | approved | rejected | auto_approved
	Reason       string
	ReviewedBy   string // dept lead agent ID who reviewed
	ReviewReason string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// BorrowRequestReader provides read access to borrow requests.
type BorrowRequestReader interface {
	GetBorrowRequest(ctx context.Context, id string) (BorrowRequest, error)
	ListPendingBorrowRequests(ctx context.Context, deptID string) ([]BorrowRequest, error)
	ListBorrowRequestsByTeam(ctx context.Context, teamID string) ([]BorrowRequest, error)
	ListExpiredPendingBorrowRequests(ctx context.Context) ([]BorrowRequest, error)
}

// BorrowRequestWriter provides write access to borrow requests.
type BorrowRequestWriter interface {
	CreateBorrowRequest(ctx context.Context, r BorrowRequest) (BorrowRequest, error)
	// CreateBorrowRequestsBulk inserts multiple borrow requests in one statement.
	CreateBorrowRequestsBulk(ctx context.Context, rs []BorrowRequest) ([]BorrowRequest, error)
	UpdateBorrowRequest(ctx context.Context, r BorrowRequest) (BorrowRequest, error)
	CancelBorrowRequestsByFromDept(ctx context.Context, deptID string) (int, error)
}

// BorrowRequestRepo combines read and write access for borrow requests.
type BorrowRequestRepo interface {
	BorrowRequestReader
	BorrowRequestWriter
}

// SubmitBorrowRequest creates a new borrow request for cross-department agent usage.
func (m *DeptLeadManager) SubmitBorrowRequest(ctx context.Context, r BorrowRequest) (BorrowRequest, error) {
	if r.TeamID == "" {
		return BorrowRequest{}, apierror.BadRequest("BORROW", "team_id is required")
	}
	if r.AgentID == "" {
		return BorrowRequest{}, apierror.BadRequest("BORROW", "agent_id is required")
	}
	if r.FromDeptID == "" {
		return BorrowRequest{}, apierror.BadRequest("BORROW", "from_dept_id is required")
	}
	if r.ToDeptID == "" {
		return BorrowRequest{}, apierror.BadRequest("BORROW", "to_dept_id is required")
	}
	if r.FromDeptID == r.ToDeptID {
		return BorrowRequest{}, apierror.BadRequest("BORROW", "from_dept_id and to_dept_id must be different")
	}
	if r.ID == "" {
		r.ID = newAgentCatalogID()
	}
	r.Status = BorrowRequestPending
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now
	return m.borrowRepo.CreateBorrowRequest(ctx, r)
}

// SubmitBorrowRequests validates and creates multiple borrow requests in one
// batch (single INSERT) to avoid N+1 on spirit team assembly.
func (m *DeptLeadManager) SubmitBorrowRequests(ctx context.Context, rs []BorrowRequest) ([]BorrowRequest, error) {
	if len(rs) == 0 {
		return nil, nil
	}
	now := time.Now().UTC()
	for i := range rs {
		r := &rs[i]
		if r.TeamID == "" || r.AgentID == "" || r.FromDeptID == "" || r.ToDeptID == "" {
			return nil, apierror.BadRequest("BORROW", "team_id/agent_id/from_dept_id/to_dept_id are required")
		}
		if r.FromDeptID == r.ToDeptID {
			return nil, apierror.BadRequest("BORROW", "from_dept_id and to_dept_id must be different")
		}
		if r.ID == "" {
			r.ID = newAgentCatalogID()
		}
		r.Status = BorrowRequestPending
		r.CreatedAt = now
		r.UpdatedAt = now
	}
	return m.borrowRepo.CreateBorrowRequestsBulk(ctx, rs)
}

// ApproveBorrowRequest approves a pending borrow request.
func (m *DeptLeadManager) ApproveBorrowRequest(ctx context.Context, id string, reviewerAgentID string, reason string) (BorrowRequest, error) {
	r, err := m.borrowRepo.GetBorrowRequest(ctx, id)
	if err != nil {
		return BorrowRequest{}, err
	}
	if r.Status != BorrowRequestPending {
		return BorrowRequest{}, apierror.BadRequest("BORROW", "only pending requests can be approved")
	}
	r.Status = BorrowRequestApproved
	r.ReviewedBy = reviewerAgentID
	r.ReviewReason = reason
	r.UpdatedAt = time.Now().UTC()
	updated, err := m.borrowRepo.UpdateBorrowRequest(ctx, r)
	if err != nil {
		return BorrowRequest{}, err
	}
	m.publishBorrowEvent("borrow.approved", "Borrow request approved", updated)
	return updated, nil
}

// RejectBorrowRequest rejects a pending borrow request.
func (m *DeptLeadManager) RejectBorrowRequest(ctx context.Context, id string, reviewerAgentID string, reason string) (BorrowRequest, error) {
	r, err := m.borrowRepo.GetBorrowRequest(ctx, id)
	if err != nil {
		return BorrowRequest{}, err
	}
	if r.Status != BorrowRequestPending {
		return BorrowRequest{}, apierror.BadRequest("BORROW", "only pending requests can be rejected")
	}
	r.Status = BorrowRequestRejected
	r.ReviewedBy = reviewerAgentID
	r.ReviewReason = reason
	r.UpdatedAt = time.Now().UTC()
	updated, err := m.borrowRepo.UpdateBorrowRequest(ctx, r)
	if err != nil {
		return BorrowRequest{}, err
	}
	m.publishBorrowEvent("borrow.rejected", "Borrow request rejected", updated)
	return updated, nil
}

// AutoApproveExpiredBorrowRequests finds all pending borrow requests that have
// exceeded BorrowAutoApproveTimeout and auto-approves them atomically.
// Returns the number of auto-approved requests.
func (m *DeptLeadManager) AutoApproveExpiredBorrowRequests(ctx context.Context) (int, error) {
	expired, err := m.borrowRepo.ListExpiredPendingBorrowRequests(ctx)
	if err != nil {
		return 0, err
	}
	if len(expired) == 0 {
		return 0, nil
	}
	approved := 0
	for _, r := range expired {
		r.Status = BorrowRequestAutoApproved
		r.ReviewReason = "auto-approved: borrow request exceeded 5-minute timeout"
		r.UpdatedAt = time.Now().UTC()
		if _, updateErr := m.borrowRepo.UpdateBorrowRequest(ctx, r); updateErr != nil {
			m.lg.Warn("failed to auto-approve expired borrow request",
				loggateway.StepID("dept_lead.auto_approve"),
				loggateway.Str("request_id", r.ID),
				loggateway.Err(updateErr))
			continue
		}
		m.publishBorrowEvent("borrow.auto_approved", "Borrow request auto-approved", r)
		approved++
	}
	return approved, nil
}

// CancelBorrowRequestsByFromDept cancels all pending borrow requests from a
// given department. Used during department deletion cascade.
func (m *DeptLeadManager) CancelBorrowRequestsByFromDept(ctx context.Context, deptID string) (int, error) {
	if m.borrowRepo == nil {
		return 0, nil
	}
	return m.borrowRepo.CancelBorrowRequestsByFromDept(ctx, deptID)
}

// ValidateBorrowRatio checks that cross-department members do not exceed 50%
// of the total enabled members in a team.
// deptID is the team's home department; agentIDs are the enabled member agent IDs.
func (m *DeptLeadManager) ValidateBorrowRatio(ctx context.Context, deptID string, agentIDs []string) error {
	if deptID == "" || len(agentIDs) == 0 {
		return nil
	}
	total := len(agentIDs)
	// Batch-fetch all agents in a single query to avoid N+1 (S1 fix).
	agents, err := m.agentRepo.ListAgentsByIDs(ctx, agentIDs)
	if err != nil {
		m.lg.Warn("failed to batch-fetch agents for borrow ratio validation",
			loggateway.StepID("dept_lead.borrow_ratio"),
			loggateway.Int("agent_count", total),
			loggateway.Err(err))
		// Fallback: skip validation rather than block the operation.
		return nil
	}
	// Collect unique non-empty position IDs for batch org-node lookup.
	positionToAgent := make(map[string]string, len(agents))
	positionIDs := make([]string, 0, len(agents))
	for _, a := range agents {
		if a.PositionID == "" {
			continue
		}
		if _, exists := positionToAgent[a.PositionID]; !exists {
			positionToAgent[a.PositionID] = a.ID
			positionIDs = append(positionIDs, a.PositionID)
		}
	}
	// Batch-fetch org nodes for all positions.
	posNodes, err := m.orgRepo.ListOrgNodesByIDs(ctx, positionIDs)
	if err != nil {
		m.lg.Warn("failed to batch-fetch org nodes for borrow ratio validation",
			loggateway.StepID("dept_lead.borrow_ratio"),
			loggateway.Err(err))
		return nil
	}
	posByID := make(map[string]OrganizationNode, len(posNodes))
	for _, p := range posNodes {
		posByID[p.ID] = p
	}
	// For each agent, resolve its department ID from the position tree.
	agentDept := make(map[string]string, len(agents))
	for _, a := range agents {
		if a.PositionID == "" {
			continue
		}
		pos, ok := posByID[a.PositionID]
		if !ok {
			continue
		}
		if pos.Level == "department" {
			agentDept[a.ID] = pos.ID
			continue
		}
		if pos.ParentID != "" {
			parent, ok := posByID[pos.ParentID]
			if ok && parent.Level == "department" {
				agentDept[a.ID] = parent.ID
			}
		}
	}
	crossDept := 0
	for _, aid := range agentIDs {
		dept, ok := agentDept[aid]
		if !ok {
			continue
		}
		if dept != "" && dept != deptID {
			crossDept++
		}
	}
	// S-09 fix: use float comparison for accurate 50% threshold
	if float64(crossDept) > float64(total)*maxCrossDeptRatio {
		return apierror.BadRequest("BORROW_RATIO", "cross-department members (%d) exceed 50%% of total members (%d)", crossDept, total)
	}
	return nil
}

// agentDepartment returns the department ID for an agent by looking up its
// position's parent department in the org tree.
func (m *DeptLeadManager) agentDepartment(ctx context.Context, agentID string) (string, error) {
	a, err := m.agentRepo.GetAgentByID(ctx, agentID)
	if err != nil {
		return "", err
	}
	if a.PositionID == "" {
		return "", nil
	}
	pos, err := m.orgRepo.GetOrgNode(ctx, a.PositionID)
	if err != nil {
		return "", err
	}
	if pos.Level == "department" {
		return pos.ID, nil
	}
	if pos.ParentID != "" {
		parent, err := m.orgRepo.GetOrgNode(ctx, pos.ParentID)
		if err != nil {
			return "", err
		}
		if parent.Level == "department" {
			return parent.ID, nil
		}
	}
	return "", nil
}

// publishBorrowEvent publishes a borrow request event as a v2 SystemNoticeEvent
// (replaces the legacy system-domain ActivityEvent; NOT persisted, WS-only broadcast).
// Uses context.Background() intentionally: event publishing is fire-and-forget
// and should not be cancelled when the originating request context expires.
func (m *DeptLeadManager) publishBorrowEvent(eventType, content string, r BorrowRequest) {
	if m.eventBus == nil {
		return
	}
	meta := map[string]any{
		"event_type":        eventType,
		"borrow_request_id": r.ID,
		"agent_id":          r.AgentID,
		"from_dept_id":      r.FromDeptID,
		"to_dept_id":        r.ToDeptID,
		"team_id":           r.TeamID,
		"status":            r.Status,
		"reviewed_by":       r.ReviewedBy,
		"review_reason":     r.ReviewReason,
	}
	m.eventBus.Publish(context.Background(), NewSystemNoticeEvent("", eventType, content, meta))
}

// BorrowedMemberStatus represents the read-only status of a borrowed member.
type BorrowedMemberStatus struct {
	AgentID        string `json:"agent_id"`
	AgentName      string `json:"agent_name"`
	TargetTeamID   string `json:"target_team_id"`
	TargetTeamName string `json:"target_team_name"`
	TeamStatus     string `json:"team_status"`
	BorrowStatus   string `json:"borrow_status"`
}

// GetBorrowedMemberStatus returns the status of agents borrowed from the given department.
// This provides a read-only view for department leads to monitor their borrowed members.
func (m *DeptLeadManager) GetBorrowedMemberStatus(ctx context.Context, deptID string) ([]BorrowedMemberStatus, error) {
	requests, err := m.borrowRepo.ListPendingBorrowRequests(ctx, deptID)
	if err != nil {
		return nil, err
	}
	var statuses []BorrowedMemberStatus
	for _, r := range requests {
		agent, aErr := m.agentRepo.GetAgentByID(ctx, r.AgentID)
		agentName := r.AgentID
		if aErr == nil {
			agentName = agent.DisplayName
		}
		status := BorrowedMemberStatus{
			AgentID:      r.AgentID,
			AgentName:    agentName,
			TargetTeamID: r.TeamID,
			BorrowStatus: r.Status,
		}
		if m.teamGetter != nil {
			if team, tErr := m.teamGetter.GetTeamByID(ctx, r.TeamID); tErr == nil {
				status.TargetTeamName = team.DisplayName
				status.TeamStatus = string(team.Status)
			}
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}
