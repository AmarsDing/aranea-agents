package service

import (
	"context"

	v1 "aranea-agents/api/kratos/ecosystem/v1"
	"aranea-agents/internal/biz"

	"aranea-agents/pkg/apierror"
	"google.golang.org/protobuf/types/known/emptypb"
)

type EcosystemService struct {
	v1.UnimplementedEcosystemServiceServer
	uc *biz.EcosystemUsecase
}

func NewEcosystemService(uc *biz.EcosystemUsecase) *EcosystemService {
	return &EcosystemService{uc: uc}
}

func productToProto(p biz.EcosystemProduct) *v1.Product {
	return &v1.Product{
		Id:           p.ID,
		Name:         p.Name,
		DisplayName:  p.DisplayName,
		Description:  p.Description,
		Type:         p.Type,
		AuthorId:     p.AuthorID,
		Version:      p.Version,
		PriceModel:   p.PriceModel,
		PriceCents:   p.PriceCents,
		Rating:       p.Rating,
		InstallCount: p.InstallCount,
		ConfigJson:   p.ConfigJSON,
		Status:       p.Status,
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
		Installed:    p.Installed,
	}
}

func (s *EcosystemService) ListProducts(ctx context.Context, req *v1.ListProductsRequest) (*v1.ListProductsResponse, error) {
	result, err := s.uc.List(ctx, biz.EcosystemQuery{
		Type:   req.GetType(),
		Search: req.GetSearch(),
		Limit:  req.GetLimit(),
		Offset: req.GetOffset(),
	})
	if err != nil {
		return nil, err
	}
	resp := &v1.ListProductsResponse{Total: result.Total}
	for i := range result.Items {
		resp.Items = append(resp.Items, productToProto(result.Items[i]))
	}
	return resp, nil
}

func (s *EcosystemService) GetProduct(ctx context.Context, req *v1.GetProductRequest) (*v1.Product, error) {
	p, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return nil, err
		}
		return nil, err
	}
	out := productToProto(p)
	return out, nil
}

func (s *EcosystemService) PublishProduct(ctx context.Context, req *v1.PublishProductRequest) (*v1.Product, error) {
	p, err := s.uc.Publish(ctx, biz.EcosystemProduct{
		Name:        req.GetName(),
		DisplayName: req.GetDisplayName(),
		Description: req.GetDescription(),
		Type:        req.GetType(),
		Version:     req.GetVersion(),
		PriceModel:  req.GetPriceModel(),
		PriceCents:  req.GetPriceCents(),
		ConfigJSON:  req.GetConfigJson(),
	})
	if err != nil {
		return nil, err
	}
	return productToProto(p), nil
}

func (s *EcosystemService) InstallProduct(ctx context.Context, req *v1.InstallProductRequest) (*v1.InstallResult, error) {
	res, err := s.uc.Install(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &v1.InstallResult{
		ProductId:    res.ProductID,
		InstalledIds: res.InstalledIDs,
		Message:      res.Message,
	}, nil
}

func (s *EcosystemService) UninstallProduct(ctx context.Context, req *v1.UninstallProductRequest) (*emptypb.Empty, error) {
	if err := s.uc.Uninstall(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
