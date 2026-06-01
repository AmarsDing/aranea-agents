package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/go-kratos/kratos/v2/errors"
	kerrors "github.com/go-kratos/kratos/v2/errors"
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

type AgentCategoryTreeNode struct {
	Category AgentCategory
	Children []AgentCategoryTreeNode
}

type CategoryAncestors struct {
	Industry   AgentCategory
	Department AgentCategory
	Position   AgentCategory
}

type VariantInfo struct {
	Key   string
	Label string
}

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

var variantSafeRe = regexp.MustCompile(`^[a-z0-9_]+$`)

func variantLabel(key string) string {
	labels := map[string]string{
		"general": "通用", "code_review": "代码审查", "architect": "架构设计",
		"drafting": "正文起草", "polishing": "润色修饰", "data_driven": "数据驱动",
		"ghostwriting": "代笔", "factor": "因子研究", "backtest": "回测执行",
		"portfolio": "组合构建", "ml_alpha": "ML Alpha", "gameplay": "Gameplay",
		"performance": "性能优化", "network": "网络同步", "ux_auditor": "UX 审计",
		"type_system": "类型系统", "migration": "迁移", "optimization": "优化",
		"audit": "审计", "implementation": "实现", "outline": "大纲设计",
		"pacing": "节奏控制", "creation": "角色创建", "consistency": "一致性维护",
		"review": "审核", "compliance": "合规", "engagement": "互动运营",
		"planning": "策划", "storyboard": "分镜", "scriptwriting": "脚本编写",
		"platform_adapt": "平台适配", "editing": "剪辑", "effects": "特效",
		"template": "模板", "motion": "动效", "branding": "品牌",
		"design": "设计", "execution_algo": "执行算法", "market_making": "做市",
		"kernel_tuning": "内核调优", "network_opt": "网络优化",
		"research_platform": "研究平台", "data_pipeline": "数据管道",
		"trading_system": "交易系统", "operations": "运维", "premarket": "盘前",
		"intraday": "盘中", "bond_analysis": "债券分析", "credit_rating": "信用评级",
		"futures_strategy": "期货策略", "options_pricing": "期权定价",
		"regulatory": "合规审查", "market_risk": "市场风险", "credit_risk": "信用风险",
		"anti_money_laundering": "反洗钱", "strategic_allocation": "战略配置",
		"client_profiling": "客户画像", "portfolio_advice": "投资建议",
		"wealth_profiling": "财富画像", "product_design": "产品设计",
		"crypto_analysis": "加密分析", "hosting": "主持", "interaction": "互动",
		"script": "脚本", "analytics": "分析", "content": "内容",
		"growth": "增长", "planting": "种草", "seo": "SEO",
		"strategy": "策略", "revenue": "变现", "adapt": "适配",
		"sync": "同步", "course": "课程", "magic_system": "魔法体系",
		"geography_history": "地理历史",
	}
	if l, ok := labels[key]; ok {
		return l
	}
	return key
}

var scenarioDirFunc = func() string {
	return filepath.Join("internal", "scenario")
}

func ScenarioDir() string {
	return scenarioDirFunc()
}

type AgentCategoryReader interface {
	ListAgentCategories(ctx context.Context) ([]AgentCategory, error)
	GetAgentCategory(ctx context.Context, id string) (AgentCategory, error)
	GetAgentCategoryByKey(ctx context.Context, key string) (AgentCategory, error)
	ListAgentCategoriesByParentID(ctx context.Context, parentID string) ([]AgentCategory, error)
	ListAgentCategoriesByLevel(ctx context.Context, level string) ([]AgentCategory, error)
}

type AgentCategoryWriter interface {
	CreateAgentCategory(ctx context.Context, c AgentCategory) (AgentCategory, error)
	UpdateAgentCategory(ctx context.Context, c AgentCategory) (AgentCategory, error)
	DeleteAgentCategory(ctx context.Context, id string) error
}

type AgentCategoryRepo interface {
	AgentCategoryReader
	AgentCategoryWriter
}

type AgentCategoryUsecase struct {
	repo AgentCategoryRepo
}

func NewAgentCategoryUsecase(repo AgentCategoryRepo) *AgentCategoryUsecase {
	return &AgentCategoryUsecase{repo: repo}
}

func (u *AgentCategoryUsecase) List(ctx context.Context) ([]AgentCategory, error) {
	return u.repo.ListAgentCategories(ctx)
}

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

func (u *AgentCategoryUsecase) Get(ctx context.Context, id string) (AgentCategory, error) {
	if strings.TrimSpace(id) == "" {
		return AgentCategory{}, ErrCategoryBadRequest("id is required")
	}
	return u.repo.GetAgentCategory(ctx, id)
}

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
	if err := u.normalizeAgentCategory(ctx, &merged); err != nil {
		return AgentCategory{}, err
	}
	return u.repo.UpdateAgentCategory(ctx, merged)
}

func (u *AgentCategoryUsecase) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrCategoryBadRequest("id is required")
	}
	return u.repo.DeleteAgentCategory(ctx, id)
}

func (u *AgentCategoryUsecase) ListByLevel(ctx context.Context, level string) ([]AgentCategory, error) {
	if strings.TrimSpace(level) == "" {
		return nil, ErrCategoryBadRequest("level is required")
	}
	return u.repo.ListAgentCategoriesByLevel(ctx, level)
}

func (u *AgentCategoryUsecase) ListByParentID(ctx context.Context, parentID string) ([]AgentCategory, error) {
	if strings.TrimSpace(parentID) == "" {
		return nil, ErrCategoryBadRequest("parent_id is required")
	}
	return u.repo.ListAgentCategoriesByParentID(ctx, parentID)
}

func (u *AgentCategoryUsecase) GetByKey(ctx context.Context, key string) (AgentCategory, error) {
	if strings.TrimSpace(key) == "" {
		return AgentCategory{}, ErrCategoryBadRequest("key is required")
	}
	return u.repo.GetAgentCategoryByKey(ctx, key)
}

func (u *AgentCategoryUsecase) GetAncestors(ctx context.Context, positionID string) (CategoryAncestors, error) {
	if strings.TrimSpace(positionID) == "" {
		return CategoryAncestors{}, kerrors.BadRequest("AGENT_CATEGORY", "position_id is required")
	}
	pos, err := u.repo.GetAgentCategory(ctx, positionID)
	if err != nil {
		return CategoryAncestors{}, err
	}
	if pos.Level != "position" {
		return CategoryAncestors{}, kerrors.BadRequest("AGENT_CATEGORY", "node is not a position")
	}
	var dept AgentCategory
	if pos.ParentID != "" {
		dept, err = u.repo.GetAgentCategory(ctx, pos.ParentID)
		if err != nil {
			return CategoryAncestors{}, err
		}
	}
	var ind AgentCategory
	if dept.ParentID != "" {
		ind, err = u.repo.GetAgentCategory(ctx, dept.ParentID)
		if err != nil {
			return CategoryAncestors{}, err
		}
	}
	return CategoryAncestors{
		Industry:   ind,
		Department: dept,
		Position:   pos,
	}, nil
}

func (u *AgentCategoryUsecase) GetPositionPrompt(ctx context.Context, industryKey, positionKey, variant string) (PositionPromptResult, error) {
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

	pos, err := u.repo.GetAgentCategoryByKey(ctx, positionKey)
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

func (u *AgentCategoryUsecase) ListPositionVariants(ctx context.Context, industryKey, positionKey string) ([]VariantInfo, error) {
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
		if isErrNoRows(err) {
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
	default:
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
	sub := string(runes[:maxChars])
	if idx := strings.LastIndex(sub, "\n"); idx > maxChars/2 {
		return strings.TrimRight(sub[:idx], " \t") + "…"
	}
	return strings.TrimRight(sub, " \t") + "…"
}

func ErrCategoryBadRequest(msg string) error {
	return errors.BadRequest("AGENT_CATEGORY", msg)
}

func isErrNoRows(err error) bool {
	return err != nil && err.Error() == "sql: no rows in result set"
}
