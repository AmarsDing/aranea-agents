package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/model_catalog/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/modelcatalog"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ModelCatalogService struct {
	v1.UnimplementedModelCatalogServiceServer
	uc *biz.ModelCatalogUsecase
}

func NewModelCatalogService(uc *biz.ModelCatalogUsecase) *ModelCatalogService {
	return &ModelCatalogService{uc: uc}
}

func (s *ModelCatalogService) GetModelCatalogStatus(ctx context.Context, _ *emptypb.Empty) (*v1.ModelCatalogStatus, error) {
	view, err := s.uc.GetStatus(ctx)
	if err != nil {
		return nil, err
	}
	return toProtoCatalogStatus(view), nil
}

func (s *ModelCatalogService) GetModelCatalogPolicy(ctx context.Context, _ *emptypb.Empty) (*v1.ModelCatalogPolicy, error) {
	p, err := s.uc.GetPolicy(ctx)
	if err != nil {
		return nil, err
	}
	return toProtoCatalogPolicy(p), nil
}

func (s *ModelCatalogService) UpdateModelCatalogPolicy(ctx context.Context, req *v1.UpdateModelCatalogPolicyRequest) (*v1.ModelCatalogPolicy, error) {
	p, err := s.uc.UpdatePolicy(ctx, modelcatalog.Policy{
		SourceURL:         req.GetSourceUrl(),
		SyncPolicy:        req.GetSyncPolicy(),
		SyncIntervalHours: int(req.GetSyncIntervalHours()),
		AutoApply:         req.GetAutoApply(),
	})
	if err != nil {
		return nil, err
	}
	return toProtoCatalogPolicy(p), nil
}

func (s *ModelCatalogService) ListCatalogProviders(ctx context.Context, req *v1.ListCatalogProvidersRequest) (*v1.ListCatalogProvidersResponse, error) {
	items, total, err := s.uc.ListProviders(ctx, req.GetQ(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.CatalogProviderSummary, 0, len(items))
	for _, p := range items {
		logoURL := s.uc.ProviderLogoURL(p.ID)
		out = append(out, &v1.CatalogProviderSummary{
			Id:         p.ID,
			Name:       p.Name,
			Doc:        p.Doc,
			Npm:        p.Npm,
			Api:        p.API,
			ModelCount: int32(len(p.Models)),
			LogoUrl:    logoURL,
			LogoCached: s.uc.HasProviderLogo(ctx, p.ID),
			Env:        append([]string(nil), p.Env...),
		})
	}
	return &v1.ListCatalogProvidersResponse{Items: out, Total: int32(total)}, nil
}

func (s *ModelCatalogService) ListCatalogModels(ctx context.Context, req *v1.ListCatalogModelsRequest) (*v1.ListCatalogModelsResponse, error) {
	items, total, err := s.uc.ListModels(ctx, req.GetProviderId(), req.GetQ(), req.GetIncludeDeprecated(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.CatalogModelSummary, 0, len(items))
	for _, m := range items {
		sum := &v1.CatalogModelSummary{
			Id:            m.ID,
			Name:          m.Name,
			Status:        m.Status,
			Reasoning:     m.Reasoning,
			ToolCall:      m.ToolCall,
			Attachment:    m.Attachment,
			ContextTokens: m.Limit.Context,
			OutputTokens:  m.Limit.Output,
			OpenWeights:   m.OpenWeights,
		}
		if m.StructuredOutput != nil {
			sum.StructuredOutput = *m.StructuredOutput
		}
		if m.Temperature != nil {
			sum.Temperature = *m.Temperature
		}
		if m.Cost != nil {
			sum.InputUsdPer_1M = m.Cost.Input
			sum.OutputUsdPer_1M = m.Cost.Output
			sum.CacheReadUsdPer_1M = m.Cost.CacheRead
			sum.CacheWriteUsdPer_1M = m.Cost.CacheWrite
			sum.ReasoningUsdPer_1M = m.Cost.Reasoning
		}
		sum.ModalityInput = append(sum.ModalityInput, m.Modalities.Input...)
		sum.ModalityOutput = append(sum.ModalityOutput, m.Modalities.Output...)
		sum.Family = m.Family
		sum.Knowledge = m.Knowledge
		sum.ReleaseDate = m.ReleaseDate
		sum.LastUpdated = m.LastUpdated
		if len(m.Interleaved) > 0 && string(m.Interleaved) != "null" {
			sum.InterleavedJson = string(m.Interleaved)
		}
		out = append(out, sum)
	}
	return &v1.ListCatalogModelsResponse{Items: out, Total: int32(total)}, nil
}

func (s *ModelCatalogService) GetModelCatalogRaw(ctx context.Context, _ *emptypb.Empty) (*v1.ModelCatalogRawResponse, error) {
	pretty, n, err := s.uc.GetRawPretty(ctx)
	if err != nil {
		return nil, err
	}
	return &v1.ModelCatalogRawResponse{JsonPretty: pretty, Bytes: n}, nil
}

func (s *ModelCatalogService) SearchCatalogRaw(ctx context.Context, req *v1.SearchCatalogRawRequest) (*v1.SearchCatalogRawResponse, error) {
	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 10
	}
	lines, total, truncated, err := s.uc.SearchRaw(ctx, req.GetQ(), limit, int(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	return &v1.SearchCatalogRawResponse{
		Lines:      lines,
		Total:      int32(total),
		Offset:     req.GetOffset(),
		Truncated:  truncated,
	}, nil
}

func (s *ModelCatalogService) ListModelCatalogSyncLogs(ctx context.Context, req *v1.ListModelCatalogSyncLogsRequest) (*v1.ListModelCatalogSyncLogsResponse, error) {
	logs, err := s.uc.ListSyncLogs(ctx, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.ModelCatalogSyncLogEntry, 0, len(logs))
	for _, e := range logs {
		details, _ := json.Marshal(e)
		out = append(out, &v1.ModelCatalogSyncLogEntry{
			Id:          e.ID,
			StartedAt:   parseCatalogTS(e.StartedAt),
			FinishedAt:  parseCatalogTS(e.FinishedAt),
			Status:      e.Status,
			Message:     e.Message,
			DetailsJson: string(details),
		})
	}
	return &v1.ListModelCatalogSyncLogsResponse{Items: out}, nil
}

func (s *ModelCatalogService) SyncModelCatalog(ctx context.Context, req *v1.SyncModelCatalogRequest) (*v1.SyncModelCatalogResponse, error) {
	out, err := s.uc.Sync(ctx, req.GetDryRun())
	view, _ := s.uc.GetStatus(ctx)
	status := toProtoCatalogStatus(view)
	resp := &v1.SyncModelCatalogResponse{
		LogId:       out.Log.ID,
		Status:      status,
		ApplyErrors: append([]string(nil), out.Log.Errors...),
		ApplyFailed: out.ApplyFailed,
	}
	if err != nil {
		msg := out.Message
		if msg == "" {
			msg = err.Error()
		}
		resp.Ok = false
		resp.Message = msg
		return resp, nil
	}
	resp.Ok = true
	resp.Message = out.Message
	return resp, nil
}

func (s *ModelCatalogService) PreviewMigration(ctx context.Context, _ *emptypb.Empty) (*v1.PreviewMigrationResponse, error) {
	preview, err := s.uc.PreviewMigration(ctx)
	if err != nil {
		return nil, err
	}
	out := &v1.PreviewMigrationResponse{Items: make([]*v1.MigrationPreviewItem, 0, len(preview.Items))}
	for _, item := range preview.Items {
		out.Items = append(out.Items, &v1.MigrationPreviewItem{
			LegacyProvider:  item.LegacyProvider,
			CatalogProvider: item.CatalogProvider,
			LlmRows:         int32(item.LLMRows),
			Agents:          int32(item.Agents),
			Sessions:        int32(item.Sessions),
			EvalFields:      int32(item.Eval),
			RuntimeSettings: int32(item.RuntimeSettings),
			Skills:          int32(item.Skills),
			KnowledgeEmbed:  int32(item.KnowledgeEmbed),
			WebResearch:     int32(item.WebResearch),
		})
	}
	return out, nil
}

func (s *ModelCatalogService) GetCatalogProviderLogo(ctx context.Context, req *v1.GetCatalogProviderLogoRequest) (*v1.GetCatalogProviderLogoResponse, error) {
	body, cached, err := s.uc.GetProviderLogo(ctx, req.GetProviderId())
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return &v1.GetCatalogProviderLogoResponse{Svg: "", Cached: false}, nil
	}
	return &v1.GetCatalogProviderLogoResponse{Svg: string(body), Cached: cached}, nil
}

func (s *ModelCatalogService) GetProviderMigrationRules(ctx context.Context, _ *emptypb.Empty) (*v1.ProviderMigrationRulesResponse, error) {
	cp, _ := s.uc.GetMigrationCheckpoint(ctx)
	out := &v1.ProviderMigrationRulesResponse{
		Version:       modelcatalog.ProviderMigrationVersion,
		LastAppliedAt: cp.AppliedAt,
	}
	for _, rule := range s.uc.ListProviderMigrationRules() {
		out.Rules = append(out.Rules, &v1.ProviderMigrationRule{
			Legacy:  rule.Legacy,
			Catalog: rule.Catalog,
		})
	}
	return out, nil
}

func (s *ModelCatalogService) ApplyProviderMigration(ctx context.Context, _ *emptypb.Empty) (*v1.ApplyProviderMigrationResponse, error) {
	stats, errs, err := s.uc.ApplyProviderMigration(ctx)
	resp := &v1.ApplyProviderMigrationResponse{
		Errors: append([]string(nil), errs...),
		Totals: &v1.MigrationPreviewItem{
			Agents:          int32(stats.Agents),
			Sessions:        int32(stats.Sessions),
			EvalFields:      int32(stats.Eval),
			RuntimeSettings: int32(stats.RuntimeSettings),
			Skills:          int32(stats.Skills),
			KnowledgeEmbed:  int32(stats.KnowledgeEmbed),
			WebResearch:     int32(stats.WebResearch),
		},
	}
	if err != nil {
		resp.Ok = false
		resp.Message = err.Error()
		return resp, nil
	}
	resp.Ok = true
	resp.Message = "provider migration applied"
	return resp, nil
}

func toProtoCatalogPolicy(p modelcatalog.Policy) *v1.ModelCatalogPolicy {
	return &v1.ModelCatalogPolicy{
		SourceUrl:         p.SourceURL,
		SyncPolicy:        p.SyncPolicy,
		SyncIntervalHours: int32(p.SyncIntervalHours),
		AutoApply:         p.AutoApply,
	}
}

func toProtoCatalogStatus(view biz.ModelCatalogStatusView) *v1.ModelCatalogStatus {
	st := &v1.ModelCatalogStatus{
		Policy:          toProtoCatalogPolicy(view.Policy),
		LastSyncStatus:  view.LastSyncStatus,
		LastSyncSummary: view.LastSyncSummary,
		Etag:            view.Meta.ETag,
		ProviderCount:   int32(view.Meta.ProviderCount),
		ModelCount:      int32(view.Meta.ModelCount),
		CatalogBytes:    view.Meta.Bytes,
		CatalogLoaded:   view.CatalogLoaded,
		LocalPath:       view.LocalPath,
	}
	if t := parseCatalogTS(view.Meta.SyncedAt); t != nil {
		st.LastSyncAt = t
	}
	return st
}

func parseCatalogTS(raw string) *timestamppb.Timestamp {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil
	}
	return timestamppb.New(t)
}
