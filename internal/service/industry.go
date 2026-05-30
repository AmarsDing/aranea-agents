package service

import (
	"context"
	"encoding/json"

	pb "aranea-agents/api/kratos/industry/v1"
	"aranea-agents/internal/biz"
)

type IndustryService struct {
	pb.UnimplementedIndustryServiceServer

	industryUC   *biz.IndustryUsecase
	departmentUC *biz.DepartmentUsecase
	positionUC   *biz.PositionUsecase
}

func NewIndustryService(industryUC *biz.IndustryUsecase, departmentUC *biz.DepartmentUsecase, positionUC *biz.PositionUsecase) *IndustryService {
	return &IndustryService{
		industryUC:   industryUC,
		departmentUC: departmentUC,
		positionUC:   positionUC,
	}
}

func (s *IndustryService) ListIndustries(ctx context.Context, req *pb.ListIndustriesRequest) (*pb.ListIndustriesResponse, error) {
	result, err := s.industryUC.List(ctx, biz.IndustryListQuery{})
	if err != nil {
		return nil, err
	}
	items := make([]*pb.Industry, 0, len(result.Items))
	for _, ind := range result.Items {
		items = append(items, &pb.Industry{
			Id:          ind.ID,
			Key:         ind.Key,
			Name:        ind.Name,
			Icon:        ind.Icon,
			Description: ind.Description,
			ScenarioKey: ind.ScenarioKey,
			Enabled:     ind.Enabled,
			SortOrder:   int32(ind.SortOrder),
		})
	}
	return &pb.ListIndustriesResponse{Items: items, Total: int32(result.Total)}, nil
}

func (s *IndustryService) GetIndustry(ctx context.Context, req *pb.GetIndustryRequest) (*pb.Industry, error) {
	ind, err := s.industryUC.GetByKey(ctx, req.Key)
	if err != nil {
		return nil, err
	}
	return &pb.Industry{
		Id:          ind.ID,
		Key:         ind.Key,
		Name:        ind.Name,
		Icon:        ind.Icon,
		Description: ind.Description,
		ScenarioKey: ind.ScenarioKey,
		Enabled:     ind.Enabled,
		SortOrder:   int32(ind.SortOrder),
	}, nil
}

func (s *IndustryService) ListDepartments(ctx context.Context, req *pb.ListDepartmentsRequest) (*pb.ListDepartmentsResponse, error) {
	result, err := s.departmentUC.ListByIndustry(ctx, req.IndustryKey)
	if err != nil {
		return nil, err
	}
	items := make([]*pb.Department, 0, len(result.Items))
	for _, d := range result.Items {
		items = append(items, &pb.Department{
			Id:                 d.ID,
			Key:                d.Key,
			Name:               d.Name,
			IndustryKey:        d.IndustryKey,
			Description:        d.Description,
			ResponsibilitiesJson: d.ResponsibilitiesJSON,
			SortOrder:          int32(d.SortOrder),
		})
	}
	return &pb.ListDepartmentsResponse{Items: items, Total: int32(result.Total)}, nil
}

func (s *IndustryService) ListPositions(ctx context.Context, req *pb.ListPositionsRequest) (*pb.ListPositionsResponse, error) {
	result, err := s.positionUC.ListByDepartment(ctx, req.DepartmentKey)
	if err != nil {
		return nil, err
	}
	items := make([]*pb.Position, 0, len(result.Items))
	for _, p := range result.Items {
		skillsJSON := "[]"
		if p.SkillsRequired != nil {
			b, mErr := json.Marshal(p.SkillsRequired)
			if mErr != nil {
				skillsJSON = "[]"
			} else {
				skillsJSON = string(b)
			}
		}
		items = append(items, &pb.Position{
			Id:                  p.ID,
			Key:                 p.Key,
			Name:                p.Name,
			DepartmentKey:       p.DepartmentKey,
			Description:         p.Description,
			ResponsibilitiesJson: p.ResponsibilitiesJSON,
			SkillsRequiredJson:  skillsJSON,
			SeniorityLevel:      p.SeniorityLevel,
			SortOrder:           int32(p.SortOrder),
		})
	}
	return &pb.ListPositionsResponse{Items: items, Total: int32(result.Total)}, nil
}

func (s *IndustryService) GetPositionPrompt(ctx context.Context, req *pb.GetPositionPromptRequest) (*pb.GetPositionPromptResponse, error) {
	result, err := s.positionUC.GetPositionPrompt(ctx, req.GetIndustryKey(), req.GetPositionKey(), req.GetVariant())
	if err != nil {
		return nil, err
	}
	return &pb.GetPositionPromptResponse{
		PromptContent:         result.PromptContent,
		Variant:               result.Variant,
		PositionName:          result.PositionName,
		DepartmentName:        result.DepartmentName,
		IndustryName:          result.IndustryName,
		IndustryDescription:   result.IndustryDescription,
		DepartmentDescription: result.DepartmentDescription,
		PositionDescription:   result.PositionDescription,
		ResponsibilitiesJson:  result.ResponsibilitiesJSON,
		VariantDescription:    result.VariantDescription,
	}, nil
}

func (s *IndustryService) ListPositionVariants(ctx context.Context, req *pb.ListPositionVariantsRequest) (*pb.ListPositionVariantsResponse, error) {
	variants, err := s.positionUC.ListPositionVariants(ctx, req.GetIndustryKey(), req.GetPositionKey())
	if err != nil {
		return nil, err
	}
	return &pb.ListPositionVariantsResponse{Variants: variants}, nil
}
