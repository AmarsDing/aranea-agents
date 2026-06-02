package biz

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// PositionPromptResult holds the resolved prompt content for a position variant.
type PositionPromptResult struct {
	PromptContent         string
	Variant               string
	PositionName          string
	DepartmentName        string
	IndustryName          string
	IndustryDescription   string
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

func newRandID() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 16)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func variantLabel(v string) string {
	if v == "general" {
		return "通用"
	}
	return v
}

type TaxonomyNode struct {
	ID           string
	Key          string
	Name         string
	Description  string
	Status       string
	Enabled      bool
	SortOrder    int
	ParentID     string
	Level        string
	ScenarioKey  string
	WorkspaceID  string
	OwnerUserID  string
	IsSystem     bool
	ConfigJSON   string
	MetadataJSON string
	CreatedAt    string
	UpdatedAt    string
	DeletedAt    string
}

type TaxonomyTreeNode struct {
	Category TaxonomyNode
	Children []TaxonomyTreeNode
}

type TaxonomyAncestors struct {
	Industry   TaxonomyNode
	Department TaxonomyNode
	Position   TaxonomyNode
}

type TaxonomyReader interface {
	ListTaxonomyNodes(ctx context.Context) ([]TaxonomyNode, error)
	GetTaxonomyNode(ctx context.Context, id string) (TaxonomyNode, error)
	GetTaxonomyNodeByKey(ctx context.Context, key string) (TaxonomyNode, error)
	ListTaxonomyNodesByParentID(ctx context.Context, parentID string) ([]TaxonomyNode, error)
	ListTaxonomyNodesByLevel(ctx context.Context, level string) ([]TaxonomyNode, error)
}

type TaxonomyWriter interface {
	CreateTaxonomyNode(ctx context.Context, c TaxonomyNode) (TaxonomyNode, error)
	UpdateTaxonomyNode(ctx context.Context, c TaxonomyNode) (TaxonomyNode, error)
	DeleteTaxonomyNode(ctx context.Context, id string) error
	ReorderTaxonomyNodes(ctx context.Context, ids []string) error
}

type TaxonomyRepo interface {
	TaxonomyReader
	TaxonomyWriter
}

type TaxonomyUsecase struct {
	repo TaxonomyRepo
}

func NewTaxonomyUsecase(repo TaxonomyRepo) *TaxonomyUsecase {
	return &TaxonomyUsecase{repo: repo}
}

func (u *TaxonomyUsecase) List(ctx context.Context) ([]TaxonomyNode, error) {
	return u.repo.ListTaxonomyNodes(ctx)
}

func (u *TaxonomyUsecase) Tree(ctx context.Context) ([]TaxonomyTreeNode, error) {
	items, err := u.repo.ListTaxonomyNodes(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make(map[string]TaxonomyNode, len(items))
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
	var buildNode func(string) TaxonomyTreeNode
	buildNode = func(id string) TaxonomyTreeNode {
		n := TaxonomyTreeNode{Category: nodes[id]}
		for _, childID := range childrenByParent[id] {
			n.Children = append(n.Children, buildNode(childID))
		}
		return n
	}
	roots := make([]TaxonomyTreeNode, 0, len(rootIDs))
	for _, id := range rootIDs {
		roots = append(roots, buildNode(id))
	}
	return roots, nil
}

func (u *TaxonomyUsecase) Get(ctx context.Context, id string) (TaxonomyNode, error) {
	if strings.TrimSpace(id) == "" {
		return TaxonomyNode{}, ErrTaxonomyBadRequest("id is required")
	}
	return u.repo.GetTaxonomyNode(ctx, id)
}

func (u *TaxonomyUsecase) Create(ctx context.Context, in TaxonomyNode) (TaxonomyNode, error) {
	in.Key = strings.TrimSpace(in.Key)
	in.Name = strings.TrimSpace(in.Name)
	if in.Key == "" || in.Name == "" {
		return TaxonomyNode{}, ErrTaxonomyBadRequest("key and name are required")
	}
	if in.ID == "" {
		in.ID = newRandID()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if err := u.normalizeTaxonomy(ctx, &in); err != nil {
		return TaxonomyNode{}, err
	}
	return u.repo.CreateTaxonomyNode(ctx, in)
}

func (u *TaxonomyUsecase) Update(ctx context.Context, id string, patch TaxonomyNode) (TaxonomyNode, error) {
	if strings.TrimSpace(id) == "" {
		return TaxonomyNode{}, ErrTaxonomyBadRequest("id is required")
	}
	current, err := u.repo.GetTaxonomyNode(ctx, id)
	if err != nil {
		return TaxonomyNode{}, err
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
	if err := u.normalizeTaxonomy(ctx, &merged); err != nil {
		return TaxonomyNode{}, err
	}
	return u.repo.UpdateTaxonomyNode(ctx, merged)
}

func (u *TaxonomyUsecase) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrTaxonomyBadRequest("id is required")
	}
	return u.repo.DeleteTaxonomyNode(ctx, id)
}

func (u *TaxonomyUsecase) ListByLevel(ctx context.Context, level string) ([]TaxonomyNode, error) {
	if strings.TrimSpace(level) == "" {
		return nil, ErrTaxonomyBadRequest("level is required")
	}
	return u.repo.ListTaxonomyNodesByLevel(ctx, level)
}

func (u *TaxonomyUsecase) ListByParentID(ctx context.Context, parentID string) ([]TaxonomyNode, error) {
	if strings.TrimSpace(parentID) == "" {
		return nil, ErrTaxonomyBadRequest("parent_id is required")
	}
	return u.repo.ListTaxonomyNodesByParentID(ctx, parentID)
}

func (u *TaxonomyUsecase) GetByKey(ctx context.Context, key string) (TaxonomyNode, error) {
	if strings.TrimSpace(key) == "" {
		return TaxonomyNode{}, ErrTaxonomyBadRequest("key is required")
	}
	return u.repo.GetTaxonomyNodeByKey(ctx, key)
}

func (u *TaxonomyUsecase) Reorder(ctx context.Context, ids []string) error {
	return u.repo.ReorderTaxonomyNodes(ctx, ids)
}

func (u *TaxonomyUsecase) GetAncestors(ctx context.Context, positionID string) (TaxonomyAncestors, error) {
	if strings.TrimSpace(positionID) == "" {
		return TaxonomyAncestors{}, kerrors.BadRequest("TAXONOMY", "position_id is required")
	}
	pos, err := u.repo.GetTaxonomyNode(ctx, positionID)
	if err != nil {
		return TaxonomyAncestors{}, err
	}
	if pos.Level != "position" {
		return TaxonomyAncestors{}, kerrors.BadRequest("TAXONOMY", "node is not a position")
	}
	var dept TaxonomyNode
	if pos.ParentID != "" {
		dept, err = u.repo.GetTaxonomyNode(ctx, pos.ParentID)
		if err != nil {
			return TaxonomyAncestors{}, err
		}
	}
	var ind TaxonomyNode
	if dept.ParentID != "" {
		ind, err = u.repo.GetTaxonomyNode(ctx, dept.ParentID)
		if err != nil {
			return TaxonomyAncestors{}, err
		}
	}
	return TaxonomyAncestors{
		Industry:   ind,
		Department: dept,
		Position:   pos,
	}, nil
}

func (u *TaxonomyUsecase) GetPositionPrompt(ctx context.Context, industryKey, positionKey, variant string) (PositionPromptResult, error) {
	if industryKey == "" {
		return PositionPromptResult{}, kerrors.BadRequest("INDUSTRY", "industry_key is required")
	}
	if positionKey == "" {
		return PositionPromptResult{}, kerrors.BadRequest("POSITION", "position_key is required")
	}
	if variant == "" {
		variant = "general"
	}
	if !variantSafeRe.MatchString(variant) {
		return PositionPromptResult{}, kerrors.BadRequest("INDUSTRY", "variant contains invalid characters")
	}

	pos, err := u.repo.GetTaxonomyNodeByKey(ctx, positionKey)
	if err != nil {
		return PositionPromptResult{}, err
	}
	anc, err := u.GetAncestors(ctx, pos.ID)
	if err != nil {
		return PositionPromptResult{}, err
	}
	if anc.Industry.Key != industryKey {
		return PositionPromptResult{}, kerrors.NotFound("POSITION", "position not found in specified industry")
	}

	scenarioDir := ScenarioDir()
	promptPath := filepath.Join(scenarioDir, industryKey, "prompts", "positions", positionKey, variant+".md")
	promptBytes, readErr := os.ReadFile(promptPath)
	usedVariant := variant
	if readErr != nil {
		if variant != "general" {
			fallbackPath := filepath.Join(scenarioDir, industryKey, "prompts", "positions", positionKey, "general.md")
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
		IndustryName:          anc.Industry.Name,
		IndustryDescription:   anc.Industry.Description,
		DepartmentDescription: anc.Department.Description,
		PositionDescription:   anc.Position.Description,
		ResponsibilitiesJSON:  anc.Position.MetadataJSON,
		VariantDescription:    variantDesc,
	}, nil
}

func (u *TaxonomyUsecase) ListPositionVariants(ctx context.Context, industryKey, positionKey string) ([]VariantInfo, error) {
	if industryKey == "" || positionKey == "" {
		return nil, kerrors.BadRequest("INDUSTRY", "industry_key and position_key are required")
	}
	scenarioDir := ScenarioDir()
	dir := filepath.Join(scenarioDir, industryKey, "prompts", "positions", positionKey)
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

func (u *TaxonomyUsecase) normalizeTaxonomy(ctx context.Context, in *TaxonomyNode) error {
	if strings.TrimSpace(in.ParentID) == "" {
		in.ParentID = ""
		if in.Level == "" {
			in.Level = "industry"
		}
		if in.Level != "industry" {
			return ErrTaxonomyBadRequest("industry node must not have parent_id")
		}
		return nil
	}
	parent, err := u.repo.GetTaxonomyNode(ctx, in.ParentID)
	if err != nil {
		if isErrNoRows(err) {
			return ErrTaxonomyBadRequest("parent node not found")
		}
		return err
	}
	switch parent.Level {
	case "industry":
		if in.Level == "" {
			in.Level = "department"
		}
		if in.Level != "department" {
			return ErrTaxonomyBadRequest("industry children must be department")
		}
	case "department":
		if in.Level == "" {
			in.Level = "position"
		}
		if in.Level != "position" {
			return ErrTaxonomyBadRequest("department children must be position")
		}
	case "position":
		return ErrTaxonomyBadRequest("position node cannot have children")
	default:
		return ErrTaxonomyBadRequest("parent node level is invalid")
	}
	return nil
}

func (u *TaxonomyUsecase) BuildResponsibility(ctx context.Context, positionID string, mode string) (string, error) {
	if strings.TrimSpace(positionID) == "" {
		return "", nil
	}
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "minimized", "none":
		return "", nil
	}

	pos, err := u.repo.GetTaxonomyNode(ctx, positionID)
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
			dept, deptErr := u.repo.GetTaxonomyNode(ctx, pos.ParentID)
			if deptErr == nil && strings.TrimSpace(dept.Description) != "" {
				result = posDesc + "\n\n部门职责：" + strings.TrimSpace(dept.Description)
			}
		}
		return result, nil
	}
}

func ErrTaxonomyBadRequest(msg string) error {
	return kerrors.BadRequest("TAXONOMY", msg)
}
