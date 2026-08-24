package biz

import (
	"context"
	"path/filepath"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

func newRandID() string {
	return uuid.NewString()
}

type OrganizationNode struct {
	ID                 string
	Key                string
	Name               string
	Description        string
	Status             string
	Enabled            bool
	SortOrder          int
	ParentID           string
	Level              string
	ScenarioKey        string
	WorkspaceID        string
	OwnerUserID        string
	IsSystem           bool
	ConfigJSON         string
	MetadataJSON       string
	DeptLeadAgentID    string
	DeptLeadConfigJSON string
	// CompanyLeadAgentID is persisted in metadata_json (Evolving; dedicated
	// column may follow). Empty when the node is not a company or none linked.
	CompanyLeadAgentID string
	CreatedAt          string
	UpdatedAt          string
	DeletedAt          string
}

type OrganizationTreeNode struct {
	Category OrganizationNode
	Children []OrganizationTreeNode
}

type OrgAncestors struct {
	Company    OrganizationNode
	Department OrganizationNode
	Position   OrganizationNode
}

// OrganizationReader provides read-only access to organization nodes.
// Stability:stable
type OrganizationReader interface {
	GetOrgNode(ctx context.Context, id string) (OrganizationNode, error)
	GetOrgNodeByKey(ctx context.Context, key string) (OrganizationNode, error)
	ListOrgNodes(ctx context.Context) ([]OrganizationNode, error)
	ListOrgNodesByLevel(ctx context.Context, level string) ([]OrganizationNode, error)
	ListOrgNodesByParentID(ctx context.Context, parentID string) ([]OrganizationNode, error)
	// ListOrgNodesByIDs returns org nodes matching the given IDs in a single query.
	// Missing IDs are silently skipped.
	ListOrgNodesByIDs(ctx context.Context, ids []string) ([]OrganizationNode, error)
}

// OrganizationWriter provides write access to organization nodes.
// Stability:stable
type OrganizationWriter interface {
	CreateOrgNode(ctx context.Context, c OrganizationNode) (OrganizationNode, error)
	UpdateOrgNode(ctx context.Context, c OrganizationNode) (OrganizationNode, error)
	DeleteOrgNode(ctx context.Context, id string) error
	ReorderOrgNodes(ctx context.Context, ids []string) error
}

// OrganizationRepo composes read and write interfaces for organization nodes.
// It also includes GetOrgNodeByKeyAnyState for soft-deleted node lookups.
// Stability:stable
type OrganizationRepo interface {
	OrganizationReader
	OrganizationWriter
	GetOrgNodeByKeyAnyState(ctx context.Context, key string) (OrganizationNode, error)
}

// DeptTeamLister lists teams by department ID for cascade operations.
// Stability:stable
type DeptTeamLister interface {
	ListTeamsByDepartmentID(ctx context.Context, deptID string) ([]Team, error)
}

// DeptAgentPositionClearer clears agent position associations for a department.
// Stability:stable
type DeptAgentPositionClearer interface {
	ClearPositionByDepartment(ctx context.Context, deptID string) (int, error)
}

type OrganizationUsecase struct {
	repo        OrganizationRepo
	deptLeadMgr *DeptLeadManager
	teamLister  DeptTeamLister
	teamWriter  TeamWriter
	agentClear  DeptAgentPositionClearer
	eventBus    EventBus
	lg          loggateway.Logger
	posPrompt   *PositionPromptUsecase
}

func NewOrganizationUsecase(repo OrganizationRepo, deptLeadMgr *DeptLeadManager, teamLister DeptTeamLister, teamWriter TeamWriter, agentClear DeptAgentPositionClearer, posPrompt *PositionPromptUsecase, eventBus EventBus, lg loggateway.Logger) *OrganizationUsecase {
	return &OrganizationUsecase{repo: repo, deptLeadMgr: deptLeadMgr, teamLister: teamLister, teamWriter: teamWriter, agentClear: agentClear, eventBus: eventBus, lg: lg, posPrompt: posPrompt}
}

func (u *OrganizationUsecase) List(ctx context.Context) ([]OrganizationNode, error) {
	u.ensureCompanyLeads(ctx)
	return u.repo.ListOrgNodes(ctx)
}

func (u *OrganizationUsecase) Tree(ctx context.Context) ([]OrganizationTreeNode, error) {
	u.ensureCompanyLeads(ctx)
	items, err := u.repo.ListOrgNodes(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make(map[string]OrganizationNode, len(items))
	order := make([]string, 0, len(items))
	for _, item := range items {
		nodes[item.ID] = item
		order = append(order, item.ID)
	}
	childrenByParent := make(map[string][]string, len(items))
	rootIDs := make([]string, 0)
	for _, id := range order {
		node := nodes[id]
		if node.ParentID != "" {
			if _, ok := nodes[node.ParentID]; ok {
				childrenByParent[node.ParentID] = append(childrenByParent[node.ParentID], id)
				continue
			}
		}
		rootIDs = append(rootIDs, id)
	}
	var buildNode func(string) OrganizationTreeNode
	buildNode = func(id string) OrganizationTreeNode {
		n := OrganizationTreeNode{Category: nodes[id]}
		for _, childID := range childrenByParent[id] {
			n.Children = append(n.Children, buildNode(childID))
		}
		return n
	}
	roots := make([]OrganizationTreeNode, 0, len(rootIDs))
	for _, id := range rootIDs {
		roots = append(roots, buildNode(id))
	}
	return roots, nil
}

func (u *OrganizationUsecase) Get(ctx context.Context, id string) (OrganizationNode, error) {
	if strings.TrimSpace(id) == "" {
		return OrganizationNode{}, ErrOrgBadRequest("id is required")
	}
	return u.repo.GetOrgNode(ctx, id)
}

func (u *OrganizationUsecase) Create(ctx context.Context, in OrganizationNode) (OrganizationNode, error) {
	in.Key = strings.TrimSpace(in.Key)
	in.Name = strings.TrimSpace(in.Name)
	if in.Key == "" || in.Name == "" {
		return OrganizationNode{}, ErrOrgBadRequest("key and name are required")
	}
	if in.ID == "" {
		in.ID = newRandID()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if err := u.posPrompt.normalizeOrg(ctx, &in); err != nil {
		return OrganizationNode{}, err
	}
	created, err := u.repo.CreateOrgNode(ctx, in)
	if err != nil {
		return OrganizationNode{}, err
	}
	// Auto-create dept lead for department nodes.
	// 总经理办公室（{companyKey}_office）是 company_lead 的宿主系统部门，
	// 不另生 dept_lead（org-invariants §1）；办公室由 ensureCompanyLeadPositionOn
	// 经 repo 窄接口创建，此处仅堵手工/API 创建路径。
	// 2026-08-24 评审：后缀过滤精确化为父级公司判定，避免误伤 _office 结尾的
	// 业务部门（其此前会被静默跳过且启动 seed 同样跳过，dept_lead 永久缺失）。
	skipLead := false
	if created.ParentID != "" {
		if parent, pErr := u.repo.GetOrgNode(ctx, created.ParentID); pErr == nil {
			skipLead = IsCompanyOfficeDept(parent, created.Key)
		}
	}
	if created.Level == "department" && u.deptLeadMgr != nil && !skipLead {
		if _, dlErr := u.deptLeadMgr.CreateDeptLead(ctx, created); dlErr != nil {
			u.lg.Warn("failed to create dept lead",
				loggateway.Str("dept_id", created.ID),
				loggateway.Err(dlErr))
		}
	}
	if created.Level == "company" && u.deptLeadMgr != nil {
		if _, clErr := u.deptLeadMgr.CreateCompanyLead(ctx, created); clErr != nil {
			u.lg.Warn("failed to create company lead",
				loggateway.Str("company_id", created.ID),
				loggateway.Err(clErr))
		}
	}
	u.publishOrgEvent("organization.created", "Organization created", created)
	return created, nil
}

// ensureCompanyLeads backfills 总经理 Agent + 岗位 for every company node.
func (u *OrganizationUsecase) ensureCompanyLeads(ctx context.Context) {
	if u == nil || u.deptLeadMgr == nil {
		return
	}
	companies, err := u.repo.ListOrgNodesByLevel(ctx, "company")
	if err != nil {
		return
	}
	for _, c := range companies {
		if _, clErr := u.deptLeadMgr.CreateCompanyLead(ctx, c); clErr != nil {
			u.lg.Warn("ensure company lead failed",
				loggateway.Str("company_id", c.ID),
				loggateway.Err(clErr))
		}
	}
}

// AuthorizeCompanyPlaybook persists a GM-signed playbook on the company node.
// Management path: does not create company/department nodes.
// Calling this API is the R18 new_playbook confirmation (NeedsUserConfirm).
func (u *OrganizationUsecase) AuthorizeCompanyPlaybook(ctx context.Context, companyID string, pb Playbook) (OrganizationNode, error) {
	if u == nil || u.repo == nil {
		return OrganizationNode{}, ErrOrgBadRequest("organization repo is required")
	}
	companyID = strings.TrimSpace(companyID)
	if companyID == "" || strings.TrimSpace(pb.ID) == "" {
		return OrganizationNode{}, ErrOrgBadRequest("company id and playbook id are required")
	}
	node, err := u.repo.GetOrgNode(ctx, companyID)
	if err != nil {
		return OrganizationNode{}, err
	}
	if node.Level != "company" {
		return OrganizationNode{}, ErrOrgBadRequest("playbook can only be authorized on a company node")
	}
	HydrateCompanyLeadFromMetadata(&node)
	if NeedsUserConfirm(ConfirmInput{AuthorizingPlaybook: true}) != ConfirmNewPlaybook {
		return OrganizationNode{}, ErrOrgBadRequest("playbook authorization requires user confirmation")
	}
	AuthorizePlaybookOnCompany(&node, pb)
	updated, err := u.repo.UpdateOrgNode(ctx, node)
	if err != nil {
		return OrganizationNode{}, err
	}
	return updated, nil
}

func (u *OrganizationUsecase) Update(ctx context.Context, id string, patch OrganizationNode) (OrganizationNode, error) {
	if strings.TrimSpace(id) == "" {
		return OrganizationNode{}, ErrOrgBadRequest("id is required")
	}
	current, err := u.repo.GetOrgNode(ctx, id)
	if err != nil {
		return OrganizationNode{}, err
	}
	merged := current
	patch.ID = id
	if patch.Key != "" {
		merged.Key = patch.Key
	}
	if patch.Name != "" {
		merged.Name = patch.Name
	}
	if patch.Status != "" {
		merged.Status = patch.Status
	}
	merged.Description = patch.Description
	merged.Enabled = patch.Enabled
	merged.SortOrder = patch.SortOrder
	merged.ParentID = patch.ParentID
	merged.Level = patch.Level
	// ScenarioKey is NOT in the proto message, so patch.ScenarioKey is always "".
	// Preserve it from current to avoid clearing on every update.
	// DeptLeadAgentID and DeptLeadConfigJSON are also preserved from current
	// (via merged := current above) — they should only be mutated by DeptLeadManager.
	// P2-C: workspace_id is immutable on Update (preserve current value).
	// merged.WorkspaceID stays from `current` (line 198: `merged := current`).
	// Do NOT apply patch.WorkspaceID — clients cannot forge workspace reassignment.
	merged.OwnerUserID = patch.OwnerUserID
	merged.IsSystem = patch.IsSystem
	merged.ConfigJSON = patch.ConfigJSON
	merged.MetadataJSON = patch.MetadataJSON

	if merged.Key == "" {
		merged.Key = current.Key
	}
	if merged.Name == "" {
		merged.Name = current.Name
	}
	if merged.Status == "" {
		merged.Status = current.Status
	}
	if err := u.posPrompt.normalizeOrg(ctx, &merged); err != nil {
		return OrganizationNode{}, err
	}
	updated, err := u.repo.UpdateOrgNode(ctx, merged)
	if err != nil {
		return OrganizationNode{}, err
	}
	u.publishOrgEvent("organization.updated", "Organization updated", updated)
	return updated, nil
}

func (u *OrganizationUsecase) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrOrgBadRequest("id is required")
	}
	node, err := u.repo.GetOrgNode(ctx, id)
	if err != nil {
		return err
	}

	// Department-level deletion requires cascade handling
	if node.Level == "department" {
		err := u.deleteDepartmentWithCascade(ctx, node)
		if err == nil {
			u.publishOrgEvent("organization.deleted", "Organization deleted", node)
		}
		return err
	}

	// Non-department nodes: simple delete with lead cleanup
	if u.deptLeadMgr != nil {
		if node.Level == "company" {
			if clErr := u.deptLeadMgr.DeleteCompanyLead(ctx, id); clErr != nil {
				u.lg.Warn("failed to delete company lead",
					loggateway.Str("company_id", id),
					loggateway.Err(clErr))
			}
		} else if dlErr := u.deptLeadMgr.DeleteDeptLead(ctx, id); dlErr != nil {
			u.lg.Warn("failed to delete dept lead",
				loggateway.Str("dept_id", id),
				loggateway.Err(dlErr))
		}
	}
	err = u.repo.DeleteOrgNode(ctx, id)
	if err == nil {
		u.publishOrgEvent("organization.deleted", "Organization deleted", node)
	}
	return err
}

// deleteDepartmentWithCascade implements the full cascade logic for department deletion:
// 1. Block if there are active (running/pending) teams
// 2. Cancel pending borrow requests from this department
// 3. Archive non-active teams
// 4. Clear agent position associations
// 5. Delete dept lead agent
// 6. Delete child position nodes
// 7. Delete the department node itself
func (u *OrganizationUsecase) deleteDepartmentWithCascade(ctx context.Context, dept OrganizationNode) error {
	if err := u.cascadeBlockActiveTeams(ctx, dept); err != nil {
		return err
	}
	run := func(txCtx context.Context) error {
		u.cascadeCancelBorrowRequests(txCtx, dept)
		u.cascadeArchiveTeams(txCtx, dept)
		u.cascadeClearAgentPositions(txCtx, dept)
		u.cascadeDeleteDeptLead(txCtx, dept)
		u.cascadeDeleteChildPositions(txCtx, dept)
		return u.repo.DeleteOrgNode(txCtx, dept.ID)
	}
	if tx, ok := u.repo.(interface {
		ExecInTx(context.Context, func(context.Context) error) error
	}); ok {
		return tx.ExecInTx(ctx, run)
	}
	return run(ctx)
}

// cascadeBlockActiveTeams blocks deletion if there are active teams.
func (u *OrganizationUsecase) cascadeBlockActiveTeams(ctx context.Context, dept OrganizationNode) error {
	if u.teamLister == nil {
		return nil
	}
	teams, err := u.teamLister.ListTeamsByDepartmentID(ctx, dept.ID)
	if err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, "ORG")
	}
	for _, t := range teams {
		if t.Status == TeamStatusRunning || t.Status == TeamStatusPending {
			return apierror.BadRequest("ORG",
				"cannot delete department with active team '%s' (status: %s); please archive or cancel it first", t.DisplayName, t.Status)
		}
	}
	return nil
}

// cascadeCancelBorrowRequests cancels pending borrow requests from this department.
func (u *OrganizationUsecase) cascadeCancelBorrowRequests(ctx context.Context, dept OrganizationNode) {
	if u.deptLeadMgr == nil {
		return
	}
	cancelled, cancelErr := u.deptLeadMgr.CancelBorrowRequestsByFromDept(ctx, dept.ID)
	if cancelErr != nil {
		u.lg.Warn("failed to cancel borrow requests during department deletion",
			loggateway.Str("dept_id", dept.ID),
			loggateway.Err(cancelErr))
	} else if cancelled > 0 {
		u.lg.Info("cancelled pending borrow requests during department deletion",
			loggateway.Str("dept_id", dept.ID),
			loggateway.Int("cancelled", cancelled))
	}
}

// cascadeArchiveTeams archives non-active teams in the department.
func (u *OrganizationUsecase) cascadeArchiveTeams(ctx context.Context, dept OrganizationNode) {
	if u.teamLister == nil {
		return
	}
	teams, err := u.teamLister.ListTeamsByDepartmentID(ctx, dept.ID)
	if err != nil {
		u.lg.Warn("failed to list department teams for archival",
			loggateway.Str("dept_id", dept.ID),
			loggateway.Err(err))
		return
	}
	var toArchive []string
	for _, t := range teams {
		if t.Status != TeamStatusArchived {
			toArchive = append(toArchive, t.ID)
		}
	}
	if len(toArchive) > 0 && u.teamWriter != nil {
		if _, archiveErr := u.teamWriter.BatchArchiveTeams(ctx, toArchive); archiveErr != nil {
			u.lg.Warn("failed to archive teams during department deletion",
				loggateway.Str("dept_id", dept.ID),
				loggateway.Err(archiveErr))
		}
	}
}

// cascadeClearAgentPositions clears agent position associations in the department.
func (u *OrganizationUsecase) cascadeClearAgentPositions(ctx context.Context, dept OrganizationNode) {
	if u.agentClear == nil {
		return
	}
	cleared, clearErr := u.agentClear.ClearPositionByDepartment(ctx, dept.ID)
	if clearErr != nil {
		u.lg.Warn("failed to clear agent positions during department deletion",
			loggateway.Str("dept_id", dept.ID),
			loggateway.Err(clearErr))
	} else if cleared > 0 {
		u.lg.Info("cleared agent position associations during department deletion",
			loggateway.Str("dept_id", dept.ID),
			loggateway.Int("cleared", cleared))
	}
}

// cascadeDeleteDeptLead deletes the department lead agent.
func (u *OrganizationUsecase) cascadeDeleteDeptLead(ctx context.Context, dept OrganizationNode) {
	if u.deptLeadMgr == nil {
		return
	}
	if dlErr := u.deptLeadMgr.DeleteDeptLead(ctx, dept.ID); dlErr != nil {
		u.lg.Warn("failed to delete dept lead during department deletion",
			loggateway.Str("dept_id", dept.ID),
			loggateway.Err(dlErr))
	}
}

// cascadeDeleteChildPositions deletes child position nodes.
func (u *OrganizationUsecase) cascadeDeleteChildPositions(ctx context.Context, dept OrganizationNode) {
	positions, posErr := u.repo.ListOrgNodesByParentID(ctx, dept.ID)
	if posErr != nil {
		u.lg.Warn("failed to list positions during department deletion",
			loggateway.Str("dept_id", dept.ID),
			loggateway.Err(posErr))
		return
	}
	for _, pos := range positions {
		if delErr := u.repo.DeleteOrgNode(ctx, pos.ID); delErr != nil {
			u.lg.Warn("failed to delete position during department deletion",
				loggateway.Str("position_id", pos.ID),
				loggateway.Err(delErr))
		}
	}
}

func (u *OrganizationUsecase) ListByLevel(ctx context.Context, level string) ([]OrganizationNode, error) {
	if strings.TrimSpace(level) == "" {
		return nil, ErrOrgBadRequest("level is required")
	}
	return u.repo.ListOrgNodesByLevel(ctx, level)
}

func (u *OrganizationUsecase) ListByParentID(ctx context.Context, parentID string) ([]OrganizationNode, error) {
	if strings.TrimSpace(parentID) == "" {
		return nil, ErrOrgBadRequest("parent_id is required")
	}
	return u.repo.ListOrgNodesByParentID(ctx, parentID)
}

func (u *OrganizationUsecase) GetByKey(ctx context.Context, key string) (OrganizationNode, error) {
	if strings.TrimSpace(key) == "" {
		return OrganizationNode{}, ErrOrgBadRequest("key is required")
	}
	return u.repo.GetOrgNodeByKey(ctx, key)
}

func (u *OrganizationUsecase) Reorder(ctx context.Context, ids []string) error {
	return u.repo.ReorderOrgNodes(ctx, ids)
}

func (u *OrganizationUsecase) GetAncestors(ctx context.Context, positionID string) (OrgAncestors, error) {
	return u.posPrompt.GetAncestors(ctx, positionID)
}

func (u *OrganizationUsecase) GetPositionPrompt(ctx context.Context, companyKey, positionKey, variant string) (PositionPromptResult, error) {
	return u.posPrompt.GetPositionPrompt(ctx, companyKey, positionKey, variant)
}

func (u *OrganizationUsecase) ListPositionVariants(ctx context.Context, companyKey, positionKey string) ([]VariantInfo, error) {
	return u.posPrompt.ListPositionVariants(ctx, companyKey, positionKey)
}

func (u *OrganizationUsecase) normalizeOrg(ctx context.Context, in *OrganizationNode) error {
	return u.posPrompt.normalizeOrg(ctx, in)
}

func (u *OrganizationUsecase) BuildResponsibility(ctx context.Context, positionID string, mode string) (string, error) {
	return u.posPrompt.BuildResponsibility(ctx, positionID, mode)
}

var scenarioDirFunc = func() string {
	return filepath.Join("internal", "scenario")
}

func ScenarioDir() string {
	return scenarioDirFunc()
}

func ErrOrgBadRequest(msg string) error {
	return apierror.BadRequest("ORG", msg)
}

// publishOrgEvent publishes an organization CRUD event as a v2 SystemNoticeEvent
// (replaces the legacy system-domain ActivityEvent; NOT persisted, WS-only broadcast).
// Uses context.Background() intentionally: event publishing is fire-and-forget
// and should not be cancelled when the originating request context expires.
func (u *OrganizationUsecase) publishOrgEvent(eventType, content string, node OrganizationNode) {
	if u.eventBus == nil {
		return
	}
	meta := map[string]any{
		"event_type": eventType,
		"org_id":     node.ID,
		"org_key":    node.Key,
		"org_name":   node.Name,
		"level":      node.Level,
		"status":     node.Status,
	}
	u.eventBus.Publish(context.Background(), NewSystemNoticeEvent("", eventType, content, meta))
}
