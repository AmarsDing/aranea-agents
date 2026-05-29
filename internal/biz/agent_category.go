package biz

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"strings"
	"sync/atomic"

	"github.com/go-kratos/kratos/v2/errors"
)

var fallbackIDCtr uint64

func newRandID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		n := atomic.AddUint64(&fallbackIDCtr, 1)
		return hex.EncodeToString([]byte{byte(n >> 56), byte(n >> 48), byte(n >> 40), byte(n >> 32), byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
	}
	return hex.EncodeToString(buf)
}

// AgentCategory is a persisted agent_category_nodes row (platform "agent-categories").
type AgentCategory struct {
	ID           string
	Key          string
	Name         string
	Description  string
	Status       string
	Enabled      bool
	SortOrder    int
	ParentID     string
	Level        string
	WorkspaceID  string
	OwnerUserID  string
	IsSystem     bool
	ConfigJSON   string
	MetadataJSON string
	CreatedAt    string
	UpdatedAt    string
	DeletedAt    string
}

// AgentCategoryTreeNode is a taxonomy node with recursive children (legacy PlatformResourceTreeNode).
type AgentCategoryTreeNode struct {
	Category AgentCategory
	Children []AgentCategoryTreeNode
}

// AgentCategoryRepo lists / mutates categories.
type AgentCategoryRepo interface {
	ListAgentCategories(ctx context.Context) ([]AgentCategory, error)
	GetAgentCategory(ctx context.Context, id string) (AgentCategory, error)
	CreateAgentCategory(ctx context.Context, c AgentCategory) (AgentCategory, error)
	UpdateAgentCategory(ctx context.Context, c AgentCategory) (AgentCategory, error)
	DeleteAgentCategory(ctx context.Context, id string) error
}

// AgentCategoryUsecase implements category workflows ported from pkg/backend/platform.
type AgentCategoryUsecase struct {
	repo AgentCategoryRepo
}

// NewAgentCategoryUsecase wires usecase.
func NewAgentCategoryUsecase(repo AgentCategoryRepo) *AgentCategoryUsecase {
	return &AgentCategoryUsecase{repo: repo}
}

// List categories (flat, non-deleted).
func (u *AgentCategoryUsecase) List(ctx context.Context) ([]AgentCategory, error) {
	return u.repo.ListAgentCategories(ctx)
}

// Tree builds a forest from flat rows (same algorithm as legacy PlatformService.Tree).
func (u *AgentCategoryUsecase) Tree(ctx context.Context) ([]AgentCategoryTreeNode, error) {
	items, err := u.repo.ListAgentCategories(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make(map[string]AgentCategory, len(items))
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
	var buildNode func(string) AgentCategoryTreeNode
	buildNode = func(id string) AgentCategoryTreeNode {
		n := AgentCategoryTreeNode{Category: nodes[id]}
		for _, childID := range childrenByParent[id] {
			n.Children = append(n.Children, buildNode(childID))
		}
		return n
	}
	roots := make([]AgentCategoryTreeNode, 0, len(rootIDs))
	for _, id := range rootIDs {
		roots = append(roots, buildNode(id))
	}
	return roots, nil
}

// Get loads one category.
func (u *AgentCategoryUsecase) Get(ctx context.Context, id string) (AgentCategory, error) {
	if strings.TrimSpace(id) == "" {
		return AgentCategory{}, ErrCategoryBadRequest("id is required")
	}
	return u.repo.GetAgentCategory(ctx, id)
}

// Create validates, normalizes, inserts.
func (u *AgentCategoryUsecase) Create(ctx context.Context, in AgentCategory) (AgentCategory, error) {
	in.Key = strings.TrimSpace(in.Key)
	in.Name = strings.TrimSpace(in.Name)
	if in.Key == "" || in.Name == "" {
		return AgentCategory{}, ErrCategoryBadRequest("key and name are required")
	}
	if in.ID == "" {
		in.ID = newRandID()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if err := u.normalizeAgentCategory(ctx, &in); err != nil {
		return AgentCategory{}, err
	}
	return u.repo.CreateAgentCategory(ctx, in)
}

// Update merges like legacy PATCH (empty string keeps previous field for key/name/status).
func (u *AgentCategoryUsecase) Update(ctx context.Context, id string, patch AgentCategory) (AgentCategory, error) {
	if strings.TrimSpace(id) == "" {
		return AgentCategory{}, ErrCategoryBadRequest("id is required")
	}
	current, err := u.repo.GetAgentCategory(ctx, id)
	if err != nil {
		return AgentCategory{}, err
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
	if err := u.normalizeAgentCategory(ctx, &merged); err != nil {
		return AgentCategory{}, err
	}
	return u.repo.UpdateAgentCategory(ctx, merged)
}

// Delete soft-deletes after legacy checks (children + agent references).
func (u *AgentCategoryUsecase) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrCategoryBadRequest("id is required")
	}
	return u.repo.DeleteAgentCategory(ctx, id)
}

func (u *AgentCategoryUsecase) normalizeAgentCategory(ctx context.Context, in *AgentCategory) error {
	if strings.TrimSpace(in.ParentID) == "" {
		in.ParentID = ""
		if in.Level == "" {
			in.Level = "industry"
		}
		if in.Level != "industry" {
			return errors.BadRequest("CATEGORY_LEVEL", "industry category must not have parent_id")
		}
		return nil
	}
	parent, err := u.repo.GetAgentCategory(ctx, in.ParentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.BadRequest("CATEGORY_PARENT", "parent category not found")
		}
		return err
	}
	switch parent.Level {
	case "industry":
		if in.Level == "" {
			in.Level = "department"
		}
		if in.Level != "department" {
			return errors.BadRequest("CATEGORY_LEVEL", "industry children must be department")
		}
	case "department":
		if in.Level == "" {
			in.Level = "position"
		}
		if in.Level != "position" {
			return errors.BadRequest("CATEGORY_LEVEL", "department children must be position")
		}
	case "position":
		return errors.BadRequest("CATEGORY_LEVEL", "position category cannot have children")
	default:
		return errors.BadRequest("CATEGORY_LEVEL", "parent category level is invalid")
	}
	return nil
}

// BuildResponsibility constructs the role_responsibility string that is
// injected into the system instruction for an agent bound to a position.
// It reads the position's description (岗位职责) and, in complete mode only,
// also prepends the parent department's description. PGO-1-BIZ-04.
//
// mode behaviour:
//   - "complete" / "": full position desc + optional dept desc
//   - "task":          position desc truncated to 300 chars
//   - "minimized" / "none" / other: empty string (not injected)
func (u *AgentCategoryUsecase) BuildResponsibility(ctx context.Context, positionID string, mode string) (string, error) {
	if strings.TrimSpace(positionID) == "" {
		return "", nil
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "minimized", "none":
		return "", nil
	}

	pos, err := u.repo.GetAgentCategory(ctx, positionID)
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
	default: // complete / ""
		result := posDesc
		if strings.TrimSpace(pos.ParentID) != "" {
			dept, deptErr := u.repo.GetAgentCategory(ctx, pos.ParentID)
			if deptErr == nil && strings.TrimSpace(dept.Description) != "" {
				result = posDesc + "\n\n部门职责：" + strings.TrimSpace(dept.Description)
			}
		}
		return result, nil
	}
}

func truncateResponsibility(s string, maxChars int) string {
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	// Truncate at last newline boundary before maxChars to avoid mid-sentence cuts.
	sub := string(runes[:maxChars])
	if idx := strings.LastIndex(sub, "\n"); idx > maxChars/2 {
		return strings.TrimRight(sub[:idx], " \t") + "…"
	}
	return strings.TrimRight(sub, " \t") + "…"
}

// ErrCategoryBadRequest maps validation messages to 400.
func ErrCategoryBadRequest(msg string) error {
	return errors.BadRequest("AGENT_CATEGORY", msg)
}
