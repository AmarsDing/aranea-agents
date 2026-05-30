package biz

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

var variantSafeRe = regexp.MustCompile(`^[a-z0-9_]+$`)

type PositionPromptResult struct {
	PromptContent       string
	Variant             string
	PositionName        string
	DepartmentName      string
	IndustryName        string
	IndustryDescription string
	DepartmentDescription string
	PositionDescription string
	ResponsibilitiesJSON string
	VariantDescription  string
}

type PositionAncestors struct {
	Industry    Industry
	Department  Department
	Position    Position
}

type PositionReader interface {
	ListPositions(ctx context.Context, q PositionListQuery) (PositionListResult, error)
	GetPositionByKey(ctx context.Context, key, departmentKey string) (Position, error)
	GetPositionWithAncestors(ctx context.Context, positionKey string) (PositionAncestors, error)
}

type PositionWriter interface {
	CreatePosition(ctx context.Context, p Position) (Position, error)
	UpsertPositionByKey(ctx context.Context, p Position) (Position, error)
}

type PositionRepository interface {
	PositionReader
	PositionWriter
}

type PositionUsecase struct {
	repo PositionRepository
}

func NewPositionUsecase(repo PositionRepository) *PositionUsecase {
	return &PositionUsecase{repo: repo}
}

func (u *PositionUsecase) ListByDepartment(ctx context.Context, departmentKey string) (PositionListResult, error) {
	if departmentKey == "" {
		return PositionListResult{}, kerrors.BadRequest("POSITION", "department_key is required")
	}
	return u.repo.ListPositions(ctx, PositionListQuery{DepartmentKey: departmentKey})
}

func (u *PositionUsecase) UpsertByKey(ctx context.Context, p Position) (Position, error) {
	if p.Key == "" || p.DepartmentKey == "" {
		return Position{}, kerrors.BadRequest("POSITION", "key and department_key are required")
	}
	return u.repo.UpsertPositionByKey(ctx, p)
}

func (u *PositionUsecase) GetWithAncestors(ctx context.Context, positionKey string) (PositionAncestors, error) {
	if positionKey == "" {
		return PositionAncestors{}, kerrors.BadRequest("POSITION", "position_key is required")
	}
	return u.repo.GetPositionWithAncestors(ctx, positionKey)
}

func (u *PositionUsecase) GetPositionPrompt(ctx context.Context, industryKey, positionKey, variant string) (PositionPromptResult, error) {
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

	anc, err := u.repo.GetPositionWithAncestors(ctx, positionKey)
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
		ResponsibilitiesJSON:  anc.Position.ResponsibilitiesJSON,
		VariantDescription:    variantDesc,
	}, nil
}

func (u *PositionUsecase) ListPositionVariants(ctx context.Context, industryKey, positionKey string) ([]string, error) {
	if industryKey == "" || positionKey == "" {
		return nil, kerrors.BadRequest("INDUSTRY", "industry_key and position_key are required")
	}
	scenarioDir := ScenarioDir()
	dir := filepath.Join(scenarioDir, industryKey, "prompts", "positions", positionKey)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}, nil
	}
	var variants []string
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
			variants = append(variants, v)
		}
	}
	if len(variants) == 0 {
		variants = []string{"general"}
	}
	return variants, nil
}

var scenarioDirFunc = func() string {
	return filepath.Join("internal", "scenario")
}

func ScenarioDir() string {
	return scenarioDirFunc()
}
