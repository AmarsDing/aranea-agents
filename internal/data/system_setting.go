package data

import (
	"context"
	"database/sql"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
)

type systemSettingRepo struct {
	data *Data
}

// NewSystemSettingRepo implements biz.SystemSettingRepo and registers DB-backed credential key resolution.
func NewSystemSettingRepo(d *Data) biz.SystemSettingRepo {
	repo := &systemSettingRepo{data: d}
	biz.SetCredentialKeyResolver(func(ctx context.Context) ([]byte, error) {
		return biz.ResolveCredentialAESKey(ctx, repo)
	})
	return repo
}

func entToBizSystemSetting(e *ent.SystemSetting) biz.SystemSetting {
	if e == nil {
		return biz.SystemSetting{}
	}
	return biz.SystemSetting{
		RootDirectory:                     e.RootDirectory,
		WorkDirectory:                     e.WorkDirectory,
		GlobalMonthlyMicroUSD:             e.GlobalMonthlyMicroUsd,
		A2APublicBaseURL:                  e.A2aPublicBaseURL,
		CredentialEncryptionKeyConfigured: strings.TrimSpace(e.CredentialEncryptionKey) != "",
		KnowledgeEmbed:                    entToKnowledgeEmbed(e),
		EvalLLM:                           entToEvalLLM(e),
		MCPAllowAdHocHTTP:                 e.McpAllowAdhocHTTP,
		UpdateTime:                        e.UpdateTime,
	}
}

func entToKnowledgeEmbed(e *ent.SystemSetting) biz.KnowledgeEmbedSetting {
	if e == nil {
		return biz.KnowledgeEmbedSetting{}
	}
	return biz.KnowledgeEmbedSetting{
		Provider:  e.KnowledgeEmbedProvider,
		BaseURL:   e.KnowledgeEmbedBaseURL,
		Model:     e.KnowledgeEmbedModel,
		Dim:       e.KnowledgeEmbedDim,
		HasAPIKey: strings.TrimSpace(e.KnowledgeEmbedAPIKey) != "",
	}
}

func entToEvalLLM(e *ent.SystemSetting) biz.EvalLLMSetting {
	if e == nil {
		return biz.EvalLLMSetting{}
	}
	return biz.EvalLLMSetting{
		SimProvider:   e.EvalSimProvider,
		SimModel:      e.EvalSimModel,
		JudgeProvider: e.EvalJudgeProvider,
		JudgeModel:    e.EvalJudgeModel,
	}
}

func (r *systemSettingRepo) Get(ctx context.Context) (biz.SystemSetting, error) {
	row, err := r.data.entClient.SystemSetting.Get(ctx, systemSettingSingletonID)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.SystemSetting{}, sql.ErrNoRows
		}
		return biz.SystemSetting{}, err
	}
	return entToBizSystemSetting(row), nil
}

func (r *systemSettingRepo) EnsureCredentialEncryptionKey(ctx context.Context) (string, error) {
	return ensureCredentialEncryptionKeyOnClient(ctx, r.data.entClient)
}

func (r *systemSettingRepo) Update(ctx context.Context, rootDir, workDir string, globalMonthlyMicroUSD int64, a2aPublicBaseURL string, mcpAllowAdHocHTTP bool) (biz.SystemSetting, error) {
	row, err := r.data.entClient.SystemSetting.UpdateOneID(systemSettingSingletonID).
		SetRootDirectory(rootDir).
		SetWorkDirectory(workDir).
		SetGlobalMonthlyMicroUsd(globalMonthlyMicroUSD).
		SetA2aPublicBaseURL(a2aPublicBaseURL).
		SetMcpAllowAdhocHTTP(mcpAllowAdHocHTTP).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.SystemSetting{}, sql.ErrNoRows
		}
		return biz.SystemSetting{}, err
	}
	return entToBizSystemSetting(row), nil
}

func (r *systemSettingRepo) GetKnowledgeEmbed(ctx context.Context) (biz.KnowledgeEmbedSetting, error) {
	row, err := r.data.entClient.SystemSetting.Get(ctx, systemSettingSingletonID)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.KnowledgeEmbedSetting{}, sql.ErrNoRows
		}
		return biz.KnowledgeEmbedSetting{}, err
	}
	out := entToKnowledgeEmbed(row)
	out.APIKey = row.KnowledgeEmbedAPIKey
	return out, nil
}

func (r *systemSettingRepo) UpdateKnowledgeEmbed(ctx context.Context, patch biz.KnowledgeEmbedSetting, updateAPIKey bool) (biz.KnowledgeEmbedSetting, error) {
	up := r.data.entClient.SystemSetting.UpdateOneID(systemSettingSingletonID).
		SetKnowledgeEmbedProvider(patch.Provider).
		SetKnowledgeEmbedBaseURL(patch.BaseURL).
		SetKnowledgeEmbedModel(patch.Model).
		SetKnowledgeEmbedDim(patch.Dim)
	if updateAPIKey && strings.TrimSpace(patch.APIKey) != "" {
		up = up.SetKnowledgeEmbedAPIKey(strings.TrimSpace(patch.APIKey))
	}
	row, err := up.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.KnowledgeEmbedSetting{}, sql.ErrNoRows
		}
		return biz.KnowledgeEmbedSetting{}, err
	}
	return entToKnowledgeEmbed(row), nil
}

func (r *systemSettingRepo) UpdateEvalLLM(ctx context.Context, patch biz.EvalLLMSetting) (biz.EvalLLMSetting, error) {
	row, err := r.data.entClient.SystemSetting.UpdateOneID(systemSettingSingletonID).
		SetEvalSimProvider(patch.SimProvider).
		SetEvalSimModel(patch.SimModel).
		SetEvalJudgeProvider(patch.JudgeProvider).
		SetEvalJudgeModel(patch.JudgeModel).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.EvalLLMSetting{}, sql.ErrNoRows
		}
		return biz.EvalLLMSetting{}, err
	}
	return entToEvalLLM(row), nil
}
