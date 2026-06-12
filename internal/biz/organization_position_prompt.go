// Package biz — Organization Position Prompt sub-module.
//
// PositionPromptUsecase is responsible for:
//   - Building position prompts from organization hierarchy (GetPositionPrompt)
//   - Listing position prompt variants (ListPositionVariants)
//   - Constructing responsibility descriptions (BuildResponsibility)
//   - Normalizing organization data for prompt consumption (normalizeOrg)
//
// It was extracted from OrganizationUsecase to separate prompt-building concerns
// from CRUD and cascade-delete operations, following SRP.
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

func variantLabel(v string) string {
	if v == "general" {
		return "通用"
	}
	return v
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

// PositionPromptUsecase handles position prompt resolution, variant listing,
// responsibility building, and org normalization — responsibilities that were
// previously mixed into OrganizationUsecase.
type PositionPromptUsecase struct {
	repo OrganizationRepo
	lg   loggateway.Logger
}

func NewPositionPromptUsecase(repo OrganizationRepo, lg loggateway.Logger) *PositionPromptUsecase {
	return &PositionPromptUsecase{repo: repo, lg: lg}
}

func (u *PositionPromptUsecase) GetPositionPrompt(ctx context.Context, companyKey, positionKey, variant string) (PositionPromptResult, error) {
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

func (u *PositionPromptUsecase) ListPositionVariants(ctx context.Context, companyKey, positionKey string) ([]VariantInfo, error) {
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

func (u *PositionPromptUsecase) BuildResponsibility(ctx context.Context, positionID string, mode string) (string, error) {
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

func (u *PositionPromptUsecase) normalizeOrg(ctx context.Context, in *OrganizationNode) error {
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

// GetAncestors resolves the company → department → position chain for a position node.
// This is used by GetPositionPrompt and can be called independently.
func (u *PositionPromptUsecase) GetAncestors(ctx context.Context, positionID string) (OrgAncestors, error) {
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
