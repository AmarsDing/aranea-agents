package service

import (
	"context"
	"database/sql"
	"errors"

	"aranea-agents/internal/event"

	v1 "aranea-agents/api/kratos/hook/v1"
	"aranea-agents/internal/biz"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/pkg/safego"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// HookService implements kratos hook.v1.
type HookService struct {
	v1.UnimplementedHookServiceServer

	uc  *biz.HookUsecase
	mgr *plugintrpc.Manager
}

func NewHookService(uc *biz.HookUsecase, mgr *plugintrpc.Manager) *HookService {
	return &HookService{uc: uc, mgr: mgr}
}

func (s *HookService) reloadHooks(ctx context.Context) {
	if s.mgr == nil {
		return
	}
	safego.Go(ctx, "hook.reload", func() {
		if err := s.mgr.ReloadHooks(context.Background()); err != nil {
			event.SysLogWarn("system.hook.reload_fail", "Hook 重载失败", event.P("error", err))
		}
	})
}

func toProtoHook(h biz.Hook) *v1.Hook {
	return &v1.Hook{
		Id:           h.ID,
		Key:          h.Key,
		Name:         h.Name,
		Description:  h.Description,
		Status:       h.Status,
		Enabled:      h.Enabled,
		SortOrder:    int32(h.SortOrder),
		ConfigJson:   h.ConfigJSON,
		MetadataJson: h.MetadataJSON,
		CreatedAt:    h.CreatedAt,
		UpdatedAt:    h.UpdatedAt,
		DeletedAt:    h.DeletedAt,
	}
}

func patchFromProtoHook(pb *v1.Hook) biz.Hook {
	if pb == nil {
		return biz.Hook{}
	}
	return biz.Hook{
		Key:          pb.GetKey(),
		Name:         pb.GetName(),
		Description:  pb.GetDescription(),
		Status:       pb.GetStatus(),
		Enabled:      pb.GetEnabled(),
		SortOrder:    int(pb.GetSortOrder()),
		ConfigJSON:   pb.GetConfigJson(),
		MetadataJSON: pb.GetMetadataJson(),
	}
}

func (s *HookService) ListHooks(ctx context.Context, _ *emptypb.Empty) (*v1.ListHooksResponse, error) {
	items, err := s.uc.List(ctx)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListHooksResponse{Items: make([]*v1.Hook, 0, len(items))}
	for i := range items {
		resp.Items = append(resp.Items, toProtoHook(items[i]))
	}
	return resp, nil
}

func (s *HookService) CreateHook(ctx context.Context, req *v1.CreateHookRequest) (*v1.Hook, error) {
	in := biz.Hook{
		Key:          req.GetKey(),
		Name:         req.GetName(),
		Description:  req.GetDescription(),
		Status:       req.GetStatus(),
		Enabled:      req.GetEnabled(),
		SortOrder:    int(req.GetSortOrder()),
		ConfigJSON:   req.GetConfigJson(),
		MetadataJSON: req.GetMetadataJson(),
	}
	out, err := s.uc.Create(ctx, in)
	if err != nil {
		return nil, err
	}
	s.reloadHooks(ctx)
	return toProtoHook(out), nil
}

func (s *HookService) GetHook(ctx context.Context, req *v1.GetHookRequest) (*v1.Hook, error) {
	h, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("HOOK", "hook not found")
		}
		return nil, err
	}
	return toProtoHook(h), nil
}

func (s *HookService) UpdateHook(ctx context.Context, req *v1.UpdateHookRequest) (*v1.Hook, error) {
	if req.GetHook() == nil {
		return nil, kerrors.BadRequest("HOOK", "hook body is required")
	}
	out, err := s.uc.Update(ctx, req.GetId(), patchFromProtoHook(req.GetHook()))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, kerrors.NotFound("HOOK", "hook not found")
		}
		return nil, err
	}
	s.reloadHooks(ctx)
	return toProtoHook(out), nil
}

func (s *HookService) DeleteHook(ctx context.Context, req *v1.DeleteHookRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, err
	}
	s.reloadHooks(ctx)
	return &emptypb.Empty{}, nil
}
