package biz

import (
	"context"
	"fmt"
	"os"
	"strings"

	"aranea-agents/internal/modelcatalog"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ModelCatalogRootResolver supplies platform root directory for catalog files.
type ModelCatalogRootResolver interface {
	GetRootDirectory(ctx context.Context) (string, error)
}

// ModelCatalogStoreProvider resolves catalog Store from dynamic root (shared by usecase + runner).
type ModelCatalogStoreProvider struct {
	roots ModelCatalogRootResolver
}

func NewModelCatalogStoreProvider(roots ModelCatalogRootResolver) *ModelCatalogStoreProvider {
	return &ModelCatalogStoreProvider{roots: roots}
}

func (p *ModelCatalogStoreProvider) Store(ctx context.Context) (*modelcatalog.Store, error) {
	root, err := p.roots.GetRootDirectory(ctx)
	if err != nil {
		return nil, err
	}
	return modelcatalog.NewStore(root), nil
}

type ModelCatalogUsecase struct {
	roots        ModelCatalogRootResolver
	storeProv    *ModelCatalogStoreProvider
	runner       *modelcatalog.Runner
	applier      *modelcatalog.Applier
	applyBackend modelcatalog.ApplyBackend
}

func NewModelCatalogUsecase(roots ModelCatalogRootResolver) *ModelCatalogUsecase {
	return &ModelCatalogUsecase{
		roots:     roots,
		storeProv: NewModelCatalogStoreProvider(roots),
	}
}

func (u *ModelCatalogUsecase) SetRunner(r *modelcatalog.Runner) {
	u.runner = r
}

func (u *ModelCatalogUsecase) SetApplier(a *modelcatalog.Applier) {
	u.applier = a
}

func (u *ModelCatalogUsecase) SetApplyBackend(b modelcatalog.ApplyBackend) {
	u.applyBackend = b
	if u.applier == nil && b != nil {
		u.applier = modelcatalog.NewApplier(b)
	}
}

func (u *ModelCatalogUsecase) store(ctx context.Context) (*modelcatalog.Store, error) {
	return u.storeProv.Store(ctx)
}

func (u *ModelCatalogUsecase) GetPolicy(ctx context.Context) (modelcatalog.Policy, error) {
	st, err := u.store(ctx)
	if err != nil {
		return modelcatalog.Policy{}, err
	}
	return st.LoadPolicy()
}

func (u *ModelCatalogUsecase) UpdatePolicy(ctx context.Context, p modelcatalog.Policy) (modelcatalog.Policy, error) {
	normalized, err := modelcatalog.NormalizePolicy(p)
	if err != nil {
		return modelcatalog.Policy{}, err
	}
	st, err := u.store(ctx)
	if err != nil {
		return modelcatalog.Policy{}, err
	}
	if err := st.SavePolicy(normalized); err != nil {
		return modelcatalog.Policy{}, err
	}
	return normalized, nil
}

type ModelCatalogStatusView struct {
	Policy          modelcatalog.Policy
	Meta            modelcatalog.Meta
	CatalogLoaded   bool
	LocalPath       string
	LastSyncStatus  string
	LastSyncSummary string
}

func (u *ModelCatalogUsecase) GetStatus(ctx context.Context) (ModelCatalogStatusView, error) {
	st, err := u.store(ctx)
	if err != nil {
		return ModelCatalogStatusView{}, err
	}
	policy, err := st.LoadPolicy()
	if err != nil {
		return ModelCatalogStatusView{}, err
	}
	meta, err := st.LoadMeta()
	if err != nil {
		return ModelCatalogStatusView{}, err
	}
	_, _, catErr := st.LoadCatalog()
	logs, _ := modelcatalog.ReadSyncLogs(st, 1)
	view := ModelCatalogStatusView{
		Policy:        policy,
		Meta:          meta,
		CatalogLoaded: catErr == nil,
		LocalPath:     st.CurrentPath(),
	}
	if len(logs) > 0 {
		view.LastSyncStatus = logs[0].Status
		view.LastSyncSummary = logs[0].Message
	}
	return view, nil
}

func (u *ModelCatalogUsecase) ListProviders(ctx context.Context, q string, limit, offset int) ([]modelcatalog.Provider, int, error) {
	st, err := u.store(ctx)
	if err != nil {
		return nil, 0, err
	}
	cat, _, err := st.LoadCatalog()
	if err != nil {
		return nil, 0, err
	}
	items := modelcatalog.ListProviders(cat, q, limit, offset)
	return items, modelcatalog.CountProviders(cat, q), nil
}

func (u *ModelCatalogUsecase) ListModels(ctx context.Context, providerID, q string, includeDeprecated bool, limit, offset int) ([]modelcatalog.Model, int, error) {
	st, err := u.store(ctx)
	if err != nil {
		return nil, 0, err
	}
	cat, _, err := st.LoadCatalog()
	if err != nil {
		return nil, 0, err
	}
	p, ok := cat[strings.TrimSpace(providerID)]
	if !ok {
		return nil, 0, nil
	}
	items := modelcatalog.ListModels(p, q, includeDeprecated, limit, offset)
	return items, modelcatalog.CountModels(p, q, includeDeprecated), nil
}

func (u *ModelCatalogUsecase) SearchRaw(ctx context.Context, q string, limit, offset int) ([]string, int, bool, error) {
	st, err := u.store(ctx)
	if err != nil {
		return nil, 0, false, err
	}
	return st.SearchCatalogBlocks(q, limit, offset)
}

func (u *ModelCatalogUsecase) GetRawPretty(ctx context.Context) (string, int64, error) {
	st, err := u.store(ctx)
	if err != nil {
		return "", 0, err
	}
	return st.LoadRawPretty()
}

func (u *ModelCatalogUsecase) ListSyncLogs(ctx context.Context, limit int) ([]modelcatalog.SyncLogEntry, error) {
	st, err := u.store(ctx)
	if err != nil {
		return nil, err
	}
	return modelcatalog.ReadSyncLogs(st, limit)
}

func (u *ModelCatalogUsecase) Sync(ctx context.Context, dryRun bool) (modelcatalog.SyncOutput, error) {
	if u.runner != nil {
		out, _, err := u.runner.SyncNow(ctx, dryRun)
		return u.finalizeSyncOutput(out, err)
	}
	st, stErr := u.store(ctx)
	if stErr != nil {
		return modelcatalog.SyncOutput{}, stErr
	}
	syncer := modelcatalog.NewSyncer(st)
	out, err := syncer.Sync(ctx, modelcatalog.SyncInput{DryRun: dryRun})
	if err != nil || dryRun || u.applier == nil {
		return u.finalizeSyncOutput(out, err)
	}
	if strings.EqualFold(strings.TrimSpace(out.Policy.AutoApply), "none") {
		return u.finalizeSyncOutput(out, err)
	}
	cat, _, loadErr := st.LoadCatalog()
	if loadErr != nil {
		out.ApplyFailed = true
		out.Log.Errors = append(out.Log.Errors, "load catalog after sync: "+loadErr.Error())
		return u.finalizeSyncOutput(out, kerrors.InternalServer("MODEL_CATALOG", fmt.Sprintf("load catalog after sync: %s", loadErr.Error())))
	}
	applyRes := u.applier.ApplyWithMigration(ctx, cat, out.Policy.AutoApply)
	out.Apply = applyRes
	out.Log.Stats.LLMRowsUpdated = applyRes.LLMRowsUpdated
	out.Log.Stats.DeprecatedDisabled = applyRes.LLMRowsDisabled
	out.Log.Stats.AgentsUpdated = applyRes.Migration.Agents
	if len(applyRes.Errors) > 0 {
		out.ApplyFailed = true
		out.Log.Errors = append(out.Log.Errors, applyRes.Errors...)
	} else if applyRes.Migration.Agents > 0 || applyRes.Migration.Sessions > 0 || applyRes.Migration.Eval > 0 ||
		applyRes.Migration.RuntimeSettings > 0 || applyRes.Migration.Skills > 0 || applyRes.Migration.KnowledgeEmbed > 0 ||
		applyRes.Migration.WebResearch > 0 {
		_ = st.SaveMigrationCheckpoint(modelcatalog.NewMigrationCheckpoint(applyRes.Migration))
	}
	return u.finalizeSyncOutput(out, nil)
}

func (u *ModelCatalogUsecase) finalizeSyncOutput(out modelcatalog.SyncOutput, syncErr error) (modelcatalog.SyncOutput, error) {
	if syncErr != nil {
		return out, syncErr
	}
	applyRes := out.Apply
	if applyRes.PricingRulesUpdated > 0 || applyRes.LLMRowsUpdated > 0 || applyRes.LLMRowsDisabled > 0 {
		out.Message += fmt.Sprintf("; applied catalog: llm=%d disabled=%d pricing=%d agents=%d sessions=%d eval=%d",
			applyRes.LLMRowsUpdated, applyRes.LLMRowsDisabled, applyRes.PricingRulesUpdated,
			applyRes.Migration.Agents, applyRes.Migration.Sessions, applyRes.Migration.Eval)
	}
	if out.ApplyFailed {
		return out, kerrors.InternalServer("MODEL_CATALOG", fmt.Sprintf("catalog apply failed: %s", strings.Join(out.Log.Errors, "; ")))
	}
	return out, nil
}

func (u *ModelCatalogUsecase) PreviewMigration(ctx context.Context) (modelcatalog.MigrationPreview, error) {
	if u.applyBackend == nil {
		return modelcatalog.MigrationPreview{}, nil
	}
	return modelcatalog.PreviewMigration(ctx, u.applyBackend)
}

func (u *ModelCatalogUsecase) ListProviderMigrationRules() []modelcatalog.ProviderMigrationRule {
	return modelcatalog.ListProviderMigrationRules()
}

func (u *ModelCatalogUsecase) GetMigrationCheckpoint(ctx context.Context) (modelcatalog.MigrationCheckpoint, error) {
	st, err := u.store(ctx)
	if err != nil {
		return modelcatalog.MigrationCheckpoint{}, err
	}
	return st.LoadMigrationCheckpoint()
}

func (u *ModelCatalogUsecase) ApplyProviderMigration(ctx context.Context) (modelcatalog.ApplyMigrationStats, []string, error) {
	if u.applyBackend == nil {
		return modelcatalog.ApplyMigrationStats{}, nil, nil
	}
	stats, errs := modelcatalog.RunProviderMigrations(ctx, u.applyBackend)
	if len(errs) > 0 {
		return stats, errs, kerrors.InternalServer("MODEL_CATALOG", fmt.Sprintf("provider migration failed: %s", strings.Join(errs, "; ")))
	}
	st, err := u.store(ctx)
	if err != nil {
		return stats, errs, err
	}
	if err := st.SaveMigrationCheckpoint(modelcatalog.NewMigrationCheckpoint(stats)); err != nil {
		return stats, errs, err
	}
	return stats, nil, nil
}

func (u *ModelCatalogUsecase) GetProviderLogo(ctx context.Context, providerID string) ([]byte, bool, error) {
	st, err := u.store(ctx)
	if err != nil {
		return nil, false, err
	}
	providerID = modelcatalog.MigrateProviderCode(strings.TrimSpace(providerID))
	if providerID == "" {
		return nil, false, nil
	}
	body, err := st.ReadProviderLogo(providerID)
	if err != nil {
		if os.IsNotExist(err) {
			body, err = st.ReadProviderLogo("default")
			if err != nil {
				if os.IsNotExist(err) {
					return nil, false, nil
				}
				return nil, false, err
			}
			return body, false, nil
		}
		return nil, false, err
	}
	return body, true, nil
}

func (u *ModelCatalogUsecase) ProviderLogoURL(providerID string) string {
	providerID = modelcatalog.MigrateProviderCode(strings.TrimSpace(providerID))
	return modelcatalog.ProviderLogoURL(providerID)
}

func (u *ModelCatalogUsecase) HasProviderLogo(ctx context.Context, providerID string) bool {
	st, err := u.store(ctx)
	if err != nil {
		return false
	}
	providerID = modelcatalog.MigrateProviderCode(strings.TrimSpace(providerID))
	return st.HasProviderLogo(providerID)
}

// systemSettingRootAdapter implements ModelCatalogRootResolver using SystemSettingRepo.
type systemSettingRootAdapter struct {
	repo SystemSettingRepo
}

func NewSystemSettingRootAdapter(repo SystemSettingRepo) ModelCatalogRootResolver {
	return &systemSettingRootAdapter{repo: repo}
}

func (a *systemSettingRootAdapter) GetRootDirectory(ctx context.Context) (string, error) {
	s, err := a.repo.Get(ctx)
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(s.RootDirectory)
	if root == "" {
		root = strings.TrimSpace(s.WorkDirectory)
	}
	return root, nil
}
