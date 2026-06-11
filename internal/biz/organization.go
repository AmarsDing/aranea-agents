package biz

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// PositionPromptResult holds the resolved prompt content for a position variant.
type PositionPromptResult struct {
	PromptContent         string
	Variant               string
	PositionName          string
	DepartmentName        string
	CompanyName           string
	CompanyDescription    string
	DepartmentDescription string
	PositionDescription   string
	ResponsibilitiesJSON  string
	VariantDescription    string
}

// VariantInfo describes a single variant of a position.
type VariantInfo struct {
	Key   string
	Label string
}

var variantSafeRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// pathSegmentSafeRe validates that a path segment (companyKey, positionKey)
// does not contain directory traversal characters. B-03 fix.
var pathSegmentSafeRe = regexp.MustCompile(`^[a-zA-Z0-9_\-.]+$`)

func newRandID() string {
	return uuid.NewString()
}

func variantLabel(v string) string {
	if v == "general" {
		return "通用"
	}
	return v
}

type OrganizationNode struct {
	ID                  string
	Key                 string
	Name                string
	Description         string
	Status              string
	Enabled             bool
	SortOrder           int
	ParentID            string
	Level               string
	ScenarioKey         string
	WorkspaceID         string
	OwnerUserID         string
	IsSystem            bool
	ConfigJSON          string
	MetadataJSON        string
	DeptLeadAgentID     string
	DeptLeadConfigJSON  string
	CreatedAt           string
	UpdatedAt           string
	DeletedAt           string
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
type OrganizationReader interface {
	GetOrgNode(ctx context.Context, id string) (OrganizationNode, error)
	GetOrgNodeByKey(ctx context.Context, key string) (OrganizationNode, error)
	ListOrgNodes(ctx context.Context) ([]OrganizationNode, error)
	ListOrgNodesByLevel(ctx context.Context, level string) ([]OrganizationNode, error)
	ListOrgNodesByParentID(ctx context.Context, parentID string) ([]OrganizationNode, error)
}

// OrganizationWriter provides write access to organization nodes.
type OrganizationWriter interface {
	CreateOrgNode(ctx context.Context, c OrganizationNode) (OrganizationNode, error)
	UpdateOrgNode(ctx context.Context, c OrganizationNode) (OrganizationNode, error)
	DeleteOrgNode(ctx context.Context, id string) error
	ReorderOrgNodes(ctx context.Context, ids []string) error
}

// OrganizationRepo composes read and write interfaces for organization nodes.
// It also includes GetOrgNodeByKeyAnyState for soft-deleted node lookups.
type OrganizationRepo interface {
	OrganizationReader
	OrganizationWriter
	GetOrgNodeByKeyAnyState(ctx context.Context, key string) (OrganizationNode, error)
}

// DeptTeamLister lists teams by department ID for cascade operations.
type DeptTeamLister interface {
	ListTeamsByDepartmentID(ctx context.Context, deptID string) ([]Team, error)
}

// DeptAgentPositionClearer clears agent position associations for a department.
type DeptAgentPositionClearer interface {
	ClearPositionByDepartment(ctx context.Context, deptID string) (int, error)
}

type OrganizationUsecase struct {
	repo        OrganizationRepo
	deptLeadMgr *DeptLeadManager
	teamLister  DeptTeamLister
	teamWriter  TeamWriter
	agentClear  DeptAgentPositionClearer
	lg          loggateway.Logger
}

func NewOrganizationUsecase(repo OrganizationRepo, deptLeadMgr *DeptLeadManager, teamLister DeptTeamLister, teamWriter TeamWriter, agentClear DeptAgentPositionClearer, lg loggateway.Logger) *OrganizationUsecase {
	return &OrganizationUsecase{repo: repo, deptLeadMgr: deptLeadMgr, teamLister: teamLister, teamWriter: teamWriter, agentClear: agentClear, lg: lg}
}

func (u *OrganizationUsecase) List(ctx context.Context) ([]OrganizationNode, error) {
	return u.repo.ListOrgNodes(ctx)
}

func (u *OrganizationUsecase) Tree(ctx context.Context) ([]OrganizationTreeNode, error) {
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
	if err := u.normalizeOrg(ctx, &in); err != nil {
		return OrganizationNode{}, err
	}
	created, err := u.repo.CreateOrgNode(ctx, in)
	if err != nil {
		return OrganizationNode{}, err
	}
	// Auto-create dept lead for department nodes
	if created.Level == "department" && u.deptLeadMgr != nil {
		if _, dlErr := u.deptLeadMgr.CreateDeptLead(ctx, created); dlErr != nil {
			u.lg.Warn("failed to create dept lead",
				loggateway.Str("dept_id", created.ID),
				loggateway.Err(dlErr))
		}
	}
	return created, nil
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
	merged.ScenarioKey = patch.ScenarioKey
	merged.WorkspaceID = patch.WorkspaceID
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
	if err := u.normalizeOrg(ctx, &merged); err != nil {
		return OrganizationNode{}, err
	}
	return u.repo.UpdateOrgNode(ctx, merged)
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
		return u.deleteDepartmentWithCascade(ctx, node)
	}

	// Non-department nodes: simple delete with dept lead cleanup
	if u.deptLeadMgr != nil {
		if dlErr := u.deptLeadMgr.DeleteDeptLead(ctx, id); dlErr != nil {
			u.lg.Warn("failed to delete dept lead",
				loggateway.Str("dept_id", id),
				loggateway.Err(dlErr))
		}
	}
	return u.repo.DeleteOrgNode(ctx, id)
}

// deleteDepartmentWithCascade implements the full cascade logic for department deletion:
// 1. Block if there are active (running/pending) teams
// 2. Cancel pending borrow requests from this department
// 3. Archive non-active teams
// 4. Clear agent position associations
// 5. Delete dept lead agent
// 6. Delete child position nodes
// 7. Delete the department node itself
//
// Note: This operation is not wrapped in a single DB transaction because:
// (a) SQLite single-writer model provides implicit serialization for writes,
// (b) the steps involve multiple tables and cross-service calls (e.g., Agent deletion)
//     that cannot be rolled back atomically, and
// (c) each step is idempotent — re-running after partial failure is safe.
// TODO(debt): wrap steps 3-7 in a transaction when migrating to PostgreSQL.
func (u *OrganizationUsecase) deleteDepartmentWithCascade(ctx context.Context, dept OrganizationNode) error {
	// S-06 fix: extracted steps into helper methods for readability
	if err := u.cascadeBlockActiveTeams(ctx, dept); err != nil {
		return err
	}
	u.cascadeCancelBorrowRequests(ctx, dept)
	u.cascadeArchiveTeams(ctx, dept)
	u.cascadeClearAgentPositions(ctx, dept)
	u.cascadeDeleteDeptLead(ctx, dept)
	u.cascadeDeleteChildPositions(ctx, dept)
	// Step 7: Delete the department node itself
	return u.repo.DeleteOrgNode(ctx, dept.ID)
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
	if strings.TrimSpace(positionID) == "" {
		return OrgAncestors{}, apierror.BadRequest("ORG", "position_id is required")
	}
	pos, err := u.repo.GetOrgNode(ctx, positionID)
	if err != nil {
		return OrgAncestors{}, err
	}
	if pos.Level != "position" {
		return OrgAncestors{}, apierror.BadRequest("ORG", "node is not a position")
	}
	var dept OrganizationNode
	if pos.ParentID != "" {
		dept, err = u.repo.GetOrgNode(ctx, pos.ParentID)
		if err != nil {
			return OrgAncestors{}, err
		}
	}
	var company OrganizationNode
	if dept.ParentID != "" {
		company, err = u.repo.GetOrgNode(ctx, dept.ParentID)
		if err != nil {
			return OrgAncestors{}, err
		}
	}
	return OrgAncestors{
		Company:    company,
		Department: dept,
		Position:   pos,
	}, nil
}

func (u *OrganizationUsecase) GetPositionPrompt(ctx context.Context, companyKey, positionKey, variant string) (PositionPromptResult, error) {
	if companyKey == "" {
		return PositionPromptResult{}, apierror.BadRequest("ORG", "company_key is required")
	}
	if !pathSegmentSafeRe.MatchString(companyKey) {
		return PositionPromptResult{}, apierror.BadRequest("ORG", "company_key contains invalid characters")
	}
	if positionKey == "" {
		return PositionPromptResult{}, apierror.BadRequest("POSITION", "position_key is required")
	}
	if !pathSegmentSafeRe.MatchString(positionKey) {
		return PositionPromptResult{}, apierror.BadRequest("ORG", "position_key contains invalid characters")
	}
	if variant == "" {
		variant = "general"
	}
	if !variantSafeRe.MatchString(variant) {
		return PositionPromptResult{}, apierror.BadRequest("ORG", "variant contains invalid characters")
	}

	pos, err := u.repo.GetOrgNodeByKey(ctx, positionKey)
	if err != nil {
		return PositionPromptResult{}, err
	}
	anc, err := u.GetAncestors(ctx, pos.ID)
	if err != nil {
		return PositionPromptResult{}, err
	}
	if anc.Company.Key != companyKey {
		return PositionPromptResult{}, apierror.NotFound("POSITION", "position not found in specified company")
	}

	scenarioDir := ScenarioDir()
	promptPath := filepath.Join(scenarioDir, companyKey, "prompts", "positions", positionKey, variant+".md")
	promptBytes, readErr := os.ReadFile(promptPath)
	usedVariant := variant
	if readErr != nil {
		if variant != "general" {
			fallbackPath := filepath.Join(scenarioDir, companyKey, "prompts", "positions", positionKey, "general.md")
			if fbBytes, fbErr := os.ReadFile(fallbackPath); fbErr == nil {
				usedVariant = "general"
				promptBytes = fbBytes
			} else {
				usedVariant = "general"
				promptBytes = nil
			}
		} else {
			promptBytes = nil
		}
	}

	promptContent := ""
	if promptBytes != nil {
		promptContent = string(promptBytes)
	}

	variantDesc := ""
	if usedVariant != "" && usedVariant != "general" {
		variantDesc = fmt.Sprintf("本岗位的 %s 方向专家", usedVariant)
	}

	return PositionPromptResult{
		PromptContent:         promptContent,
		Variant:               usedVariant,
		PositionName:          anc.Position.Name,
		DepartmentName:        anc.Department.Name,
		CompanyName:           anc.Company.Name,
		CompanyDescription:    anc.Company.Description,
		DepartmentDescription: anc.Department.Description,
		PositionDescription:   anc.Position.Description,
		ResponsibilitiesJSON:  anc.Position.MetadataJSON,
		VariantDescription:    variantDesc,
	}, nil
}

func (u *OrganizationUsecase) ListPositionVariants(ctx context.Context, companyKey, positionKey string) ([]VariantInfo, error) {
	if companyKey == "" || positionKey == "" {
		return nil, apierror.BadRequest("ORG", "company_key and position_key are required")
	}
	if !pathSegmentSafeRe.MatchString(companyKey) {
		return nil, apierror.BadRequest("ORG", "company_key contains invalid characters")
	}
	if !pathSegmentSafeRe.MatchString(positionKey) {
		return nil, apierror.BadRequest("ORG", "position_key contains invalid characters")
	}
	scenarioDir := ScenarioDir()
	dir := filepath.Join(scenarioDir, companyKey, "prompts", "positions", positionKey)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []VariantInfo{{Key: "general", Label: "通用"}}, nil
	}
	var variants []VariantInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".md" {
			continue
		}
		v := name[:len(name)-3]
		if variantSafeRe.MatchString(v) {
			variants = append(variants, VariantInfo{Key: v, Label: variantLabel(v)})
		}
	}
	if len(variants) == 0 {
		variants = []VariantInfo{{Key: "general", Label: "通用"}}
	}
	return variants, nil
}

func (u *OrganizationUsecase) normalizeOrg(ctx context.Context, in *OrganizationNode) error {
	if strings.TrimSpace(in.ParentID) == "" {
		in.ParentID = ""
		if in.Level == "" {
			in.Level = "company"
		}
		if in.Level != "company" {
			return ErrOrgBadRequest("company node must not have parent_id")
		}
		return nil
	}
	parent, err := u.repo.GetOrgNode(ctx, in.ParentID)
	if err != nil {
		if isErrNoRows(err) {
			return ErrOrgBadRequest("parent node not found")
		}
		return err
	}
	switch parent.Level {
	case "company":
		if in.Level == "" {
			in.Level = "department"
		}
		if in.Level != "department" {
			return ErrOrgBadRequest("company children must be department")
		}
	case "department":
		if in.Level == "" {
			in.Level = "position"
		}
		if in.Level != "position" {
			return ErrOrgBadRequest("department children must be position")
		}
	case "position":
		return ErrOrgBadRequest("position node cannot have children")
	default:
		return ErrOrgBadRequest("parent node level is invalid")
	}
	return nil
}

func (u *OrganizationUsecase) BuildResponsibility(ctx context.Context, positionID string, mode string) (string, error) {
	if strings.TrimSpace(positionID) == "" {
		return "", nil
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "minimized", "none":
		return "", nil
	}

	pos, err := u.repo.GetOrgNode(ctx, positionID)
	if err != nil {
		return "", err
	}
	if pos.Level != "position" {
		return "", nil
	}
	posDesc := strings.TrimSpace(pos.Description)
	if posDesc == "" {
		return "", nil
	}

	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "task":
		return truncateResponsibility(posDesc, 300), nil
	default:
		result := posDesc
		if strings.TrimSpace(pos.ParentID) != "" {
			dept, deptErr := u.repo.GetOrgNode(ctx, pos.ParentID)
			if deptErr == nil && strings.TrimSpace(dept.Description) != "" {
				result = posDesc + "\n\n部门职责：" + strings.TrimSpace(dept.Description)
			}
		}
		return result, nil
	}
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

func isErrNoRows(err error) bool {
	return errors.Is(err, shared.ErrNotFound)
}

func truncateResponsibility(s string, maxChars int) string {
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	sub := string(runes[:maxChars])
	if idx := strings.LastIndex(sub, "\n"); idx > maxChars/2 {
		return strings.TrimRight(sub[:idx], " \t") + "…"
	}
	return strings.TrimRight(sub, " \t") + "…"
}
