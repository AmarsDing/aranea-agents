package biz

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
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
	EventBus   contract.Bus
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
	eventBus   contract.Bus
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

// CreateDeptLead creates a department lead Agent for the given department.
// Called automatically when a new department is created.
func (m *DeptLeadManager) CreateDeptLead(ctx context.Context, deptNode OrganizationNode) (*Agent, error) {
	if deptNode.Level != "department" {
		return nil, kerrors.BadRequest("DEPT_LEAD", "can only create lead for department level nodes")
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
		PositionID:   "",
		PositionKey:  "",
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
	settings := withSettingDefaults(AgentRuntimeSettings{AgentID: agent.ID})
	settings.MemoryEnabled = true
	settings.ToolsEnabled = true

	// Build config_json from settings
	configJSON, cfgErr := configJSONFromSettings(settings, agent.Files)
	if cfgErr != nil {
		return nil, kerrors.InternalServer("DEPT_LEAD", "failed to build dept lead config: "+cfgErr.Error())
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
		return nil, kerrors.NotFound("DEPT_LEAD", "department has no lead agent")
	}
	a, err := m.agentUC.Get(ctx, node.DeptLeadAgentID)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// buildDeptLeadPrompt generates the system prompt for a department lead.
func (m *DeptLeadManager) buildDeptLeadPrompt(dept OrganizationNode) string {
	desc := strings.TrimSpace(dept.Description)
	if desc == "" {
		desc = "（无描述）"
	}
	return fmt.Sprintf(`你是「%s」的部门主管。你的职责：

1. **资源协调**：管理本部门的人力资源分配，审批跨部门借调请求
2. **质量把关**：审核本部门产出的交付物质量
3. **验收确认**：确认其他部门交付给本部门的工作是否满足需求

审批规则：
- 跨部门交付物需要双方主管确认（输出方质量把关 + 接收方验收确认）
- 借调成员加入其他 Team 时，你需要审批同意
- 你自动加入本部门的所有 Team
- 借调请求超过 5 分钟未处理，系统自动批准

部门信息：
- 部门名称：%s
- 部门描述：%s`, dept.Name, dept.Name, desc)
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
		return BorrowRequest{}, kerrors.BadRequest("BORROW", "team_id is required")
	}
	if r.AgentID == "" {
		return BorrowRequest{}, kerrors.BadRequest("BORROW", "agent_id is required")
	}
	if r.FromDeptID == "" {
		return BorrowRequest{}, kerrors.BadRequest("BORROW", "from_dept_id is required")
	}
	if r.ToDeptID == "" {
		return BorrowRequest{}, kerrors.BadRequest("BORROW", "to_dept_id is required")
	}
	if r.FromDeptID == r.ToDeptID {
		return BorrowRequest{}, kerrors.BadRequest("BORROW", "from_dept_id and to_dept_id must be different")
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

// ApproveBorrowRequest approves a pending borrow request.
func (m *DeptLeadManager) ApproveBorrowRequest(ctx context.Context, id string, reviewerAgentID string, reason string) (BorrowRequest, error) {
	r, err := m.borrowRepo.GetBorrowRequest(ctx, id)
	if err != nil {
		return BorrowRequest{}, err
	}
	if r.Status != BorrowRequestPending {
		return BorrowRequest{}, kerrors.BadRequest("BORROW", "only pending requests can be approved")
	}
	r.Status = BorrowRequestApproved
	r.ReviewedBy = reviewerAgentID
	r.ReviewReason = reason
	r.UpdatedAt = time.Now().UTC()
	updated, err := m.borrowRepo.UpdateBorrowRequest(ctx, r)
	if err != nil {
		return BorrowRequest{}, err
	}
	m.publishBorrowEvent(contract.EnvelopeTypeBorrowApproved, updated)
	return updated, nil
}

// RejectBorrowRequest rejects a pending borrow request.
func (m *DeptLeadManager) RejectBorrowRequest(ctx context.Context, id string, reviewerAgentID string, reason string) (BorrowRequest, error) {
	r, err := m.borrowRepo.GetBorrowRequest(ctx, id)
	if err != nil {
		return BorrowRequest{}, err
	}
	if r.Status != BorrowRequestPending {
		return BorrowRequest{}, kerrors.BadRequest("BORROW", "only pending requests can be rejected")
	}
	r.Status = BorrowRequestRejected
	r.ReviewedBy = reviewerAgentID
	r.ReviewReason = reason
	r.UpdatedAt = time.Now().UTC()
	updated, err := m.borrowRepo.UpdateBorrowRequest(ctx, r)
	if err != nil {
		return BorrowRequest{}, err
	}
	m.publishBorrowEvent(contract.EnvelopeTypeBorrowRejected, updated)
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
		m.publishBorrowEvent(contract.EnvelopeTypeBorrowAutoApproved, r)
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
// TODO(debt): current implementation queries agent department one-by-one (N+1).
// Should batch-fetch agent departments via a single query when AgentRepository
// provides a BatchGetAgentDepartments method.
func (m *DeptLeadManager) ValidateBorrowRatio(ctx context.Context, deptID string, agentIDs []string) error {
	if deptID == "" || len(agentIDs) == 0 {
		return nil
	}
	total := len(agentIDs)
	crossDept := 0
	for _, aid := range agentIDs {
		agentDeptID, err := m.agentDepartment(ctx, aid)
		if err != nil {
			m.lg.Warn("failed to query agent department for borrow ratio validation",
				loggateway.StepID("dept_lead.borrow_ratio"),
				loggateway.Str("agent_id", aid),
				loggateway.Err(err))
			continue
		}
		if agentDeptID != "" && agentDeptID != deptID {
			crossDept++
		}
	}
	// S-09 fix: use float comparison for accurate 50% threshold
	if float64(crossDept) > float64(total)*maxCrossDeptRatio {
		return kerrors.BadRequest("BORROW_RATIO", fmt.Sprintf(
			"cross-department members (%d) exceed 50%% of total members (%d)",
			crossDept, total))
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

// publishBorrowEvent publishes a borrow request event to the event bus.
// Uses context.Background() intentionally: event publishing is fire-and-forget
// and should not be cancelled when the originating request context expires.
func (m *DeptLeadManager) publishBorrowEvent(typ contract.EnvelopeType, r BorrowRequest) {
	if m.eventBus == nil {
		return
	}
	env := contract.NewEnvelope(typ, "dept_lead", "")
	env.TeamID = r.TeamID
	env.Metadata = map[string]any{
		"borrow_request_id": r.ID,
		"agent_id":          r.AgentID,
		"from_dept_id":      r.FromDeptID,
		"to_dept_id":        r.ToDeptID,
		"status":            r.Status,
		"reviewed_by":       r.ReviewedBy,
		"review_reason":     r.ReviewReason,
	}
	m.eventBus.Publish(context.Background(), env)
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
