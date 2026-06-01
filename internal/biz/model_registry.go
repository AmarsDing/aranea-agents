package biz

import (
	"context"
	"fmt"
	"os"
	"strings"

	"aranea-agents/internal/modelregistry"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type ModelRegistryRootResolver interface {
	GetRootDirectory(ctx context.Context) (string, error)
}

type ModelRegistryStoreProvider struct {
	roots ModelRegistryRootResolver
}

func NewModelRegistryStoreProvider(roots ModelRegistryRootResolver) *ModelRegistryStoreProvider {
	return &ModelRegistryStoreProvider{roots: roots}
}

func (p *ModelRegistryStoreProvider) Store(ctx context.Context) (*modelregistry.Store, error) {
	root, err := p.roots.GetRootDirectory(ctx)
	if err != nil {
		return nil, err
	}
	return modelregistry.NewStore(root, loggateway.Global()), nil
}

type ModelRegistryUsecase struct {
	roots        ModelRegistryRootResolver
	storeProv    *ModelRegistryStoreProvider
	applyBackend modelregistry.ApplyBackend
}

func NewModelRegistryUsecase(roots ModelRegistryRootResolver, backend modelregistry.ApplyBackend) *ModelRegistryUsecase {
	return &ModelRegistryUsecase{
		roots:        roots,
		storeProv:    NewModelRegistryStoreProvider(roots),
		applyBackend: backend,
	}
}

func (u *ModelRegistryUsecase) store(ctx context.Context) (*modelregistry.Store, error) {
	return u.storeProv.Store(ctx)
}

func (u *ModelRegistryUsecase) GetPolicy(ctx context.Context) (modelregistry.Policy, error) {
	st, err := u.store(ctx)
	if err != nil {
		return modelregistry.Policy{}, err
	}
	return st.LoadPolicy()
}

func (u *ModelRegistryUsecase) UpdatePolicy(ctx context.Context, p modelregistry.Policy) (modelregistry.Policy, error) {
	normalized, err := modelregistry.NormalizePolicy(p)
	if err != nil {
		return modelregistry.Policy{}, err
	}
	st, err := u.store(ctx)
	if err != nil {
		return modelregistry.Policy{}, err
	}
	if err := st.SavePolicy(normalized); err != nil {
		return modelregistry.Policy{}, err
	}
	return normalized, nil
}

type ModelRegistryStatusView struct {
	Policy          modelregistry.Policy
	Meta            modelregistry.Meta
	DirectoryLoaded bool
	LocalPath       string
	LastSyncStatus  string
	LastSyncSummary string
}

func (u *ModelRegistryUsecase) GetStatus(ctx context.Context) (ModelRegistryStatusView, error) {
	st, err := u.store(ctx)
	if err != nil {
		return ModelRegistryStatusView{}, err
	}
	policy, err := st.LoadPolicy()
	if err != nil {
		return ModelRegistryStatusView{}, err
	}
	meta, err := st.LoadMeta()
	if err != nil {
		return ModelRegistryStatusView{}, err
	}
	_, _, dirErr := st.LoadDirectory()
	logs, _ := modelregistry.ReadSyncLogs(st, 1)
	view := ModelRegistryStatusView{
		Policy:          policy,
		Meta:            meta,
		DirectoryLoaded: dirErr == nil,
		LocalPath:       st.CurrentPath(),
	}
	if len(logs) > 0 {
		view.LastSyncStatus = logs[0].Status
		view.LastSyncSummary = logs[0].Message
	}
	return view, nil
}

func (u *ModelRegistryUsecase) ListProviders(ctx context.Context, q string, limit, offset int) ([]modelregistry.Provider, int, error) {
	st, err := u.store(ctx)
	if err != nil {
		return nil, 0, err
	}
	dir, _, err := st.LoadDirectory()
	if err != nil {
		return nil, 0, err
	}
	items := modelregistry.ListProviders(dir, q, limit, offset)
	return items, modelregistry.CountProviders(dir, q), nil
}

func (u *ModelRegistryUsecase) ListModels(ctx context.Context, providerID, q string, includeDeprecated bool, limit, offset int) ([]modelregistry.Model, int, error) {
	st, err := u.store(ctx)
	if err != nil {
		return nil, 0, err
	}
	dir, _, err := st.LoadDirectory()
	if err != nil {
		return nil, 0, err
	}
	p, ok := dir[strings.TrimSpace(providerID)]
	if !ok {
		return nil, 0, nil
	}
	items := modelregistry.ListModels(p, q, includeDeprecated, limit, offset)
	return items, modelregistry.CountModels(p, q, includeDeprecated), nil
}

func (u *ModelRegistryUsecase) SearchRaw(ctx context.Context, q string, limit, offset int) ([]string, int, bool, error) {
	st, err := u.store(ctx)
	if err != nil {
		return nil, 0, false, err
	}
	return st.SearchDirectoryBlocks(q, limit, offset)
}

func (u *ModelRegistryUsecase) GetRawPretty(ctx context.Context) (string, int64, error) {
	st, err := u.store(ctx)
	if err != nil {
		return "", 0, err
	}
	return st.LoadRawPretty()
}

func (u *ModelRegistryUsecase) ListSyncLogs(ctx context.Context, limit int) ([]modelregistry.SyncLogEntry, error) {
	st, err := u.store(ctx)
	if err != nil {
		return nil, err
	}
	return modelregistry.ReadSyncLogs(st, limit)
}

func (u *ModelRegistryUsecase) Sync(ctx context.Context, dryRun bool) (modelregistry.SyncOutput, error) {
	st, stErr := u.store(ctx)
	if stErr != nil {
		return modelregistry.SyncOutput{}, stErr
	}
	syncer := modelregistry.NewSyncer(st, loggateway.Global())
	out, err := syncer.Sync(ctx, modelregistry.SyncInput{DryRun: dryRun})
	if err != nil || dryRun || u.applyBackend == nil {
		return u.finalizeSyncOutput(out, err)
	}
	if strings.EqualFold(strings.TrimSpace(out.Policy.AutoApply), "none") {
		return u.finalizeSyncOutput(out, err)
	}
	dir, _, loadErr := st.LoadDirectory()
	if loadErr != nil {
		out.ApplyFailed = true
		out.Log.Errors = append(out.Log.Errors, "load directory after sync: "+loadErr.Error())
		return u.finalizeSyncOutput(out, kerrors.InternalServer("MODEL_REGISTRY", fmt.Sprintf("load directory after sync: %s", loadErr.Error())))
	}
	applier := modelregistry.NewApplier(u.applyBackend)
	applyRes := applier.ApplyWithMigration(ctx, dir, out.Policy.AutoApply)
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
		if cpErr := st.SaveMigrationCheckpoint(modelregistry.NewMigrationCheckpoint(applyRes.Migration)); cpErr != nil {
			out.Log.Errors = append(out.Log.Errors, "save migration checkpoint: "+cpErr.Error())
		}
	}
	return u.finalizeSyncOutput(out, nil)
}

func (u *ModelRegistryUsecase) finalizeSyncOutput(out modelregistry.SyncOutput, syncErr error) (modelregistry.SyncOutput, error) {
	if syncErr != nil {
		return out, syncErr
	}
	applyRes := out.Apply
	if applyRes.PricingRulesUpdated > 0 || applyRes.LLMRowsUpdated > 0 || applyRes.LLMRowsDisabled > 0 {
		out.Message += fmt.Sprintf("; applied directory: llm=%d disabled=%d pricing=%d agents=%d sessions=%d eval=%d",
			applyRes.LLMRowsUpdated, applyRes.LLMRowsDisabled, applyRes.PricingRulesUpdated,
			applyRes.Migration.Agents, applyRes.Migration.Sessions, applyRes.Migration.Eval)
	}
	if out.ApplyFailed {
		return out, kerrors.InternalServer("MODEL_REGISTRY", fmt.Sprintf("directory apply failed: %s", strings.Join(out.Log.Errors, "; ")))
	}
	return out, nil
}

func (u *ModelRegistryUsecase) PreviewMigration(ctx context.Context) (modelregistry.MigrationPreview, error) {
	if u.applyBackend == nil {
		return modelregistry.MigrationPreview{}, nil
	}
	return modelregistry.PreviewMigration(ctx, u.applyBackend)
}

func (u *ModelRegistryUsecase) ListProviderMigrationRules() []modelregistry.ProviderMigrationRule {
	return modelregistry.ListProviderMigrationRules()
}

func (u *ModelRegistryUsecase) GetMigrationCheckpoint(ctx context.Context) (modelregistry.MigrationCheckpoint, error) {
	st, err := u.store(ctx)
	if err != nil {
		return modelregistry.MigrationCheckpoint{}, err
	}
	return st.LoadMigrationCheckpoint()
}

func (u *ModelRegistryUsecase) ApplyProviderMigration(ctx context.Context) (modelregistry.ApplyMigrationStats, []string, error) {
	if u.applyBackend == nil {
		return modelregistry.ApplyMigrationStats{}, nil, nil
	}
	stats, errs := modelregistry.RunProviderMigrations(ctx, u.applyBackend)
	if len(errs) > 0 {
		return stats, errs, kerrors.InternalServer("MODEL_REGISTRY", fmt.Sprintf("provider migration failed: %s", strings.Join(errs, "; ")))
	}
	st, err := u.store(ctx)
	if err != nil {
		return stats, errs, err
	}
	if err := st.SaveMigrationCheckpoint(modelregistry.NewMigrationCheckpoint(stats)); err != nil {
		return stats, errs, err
	}
	return stats, nil, nil
}

func (u *ModelRegistryUsecase) GetProviderLogo(ctx context.Context, providerID string) ([]byte, bool, error) {
	st, err := u.store(ctx)
	if err != nil {
		return nil, false, err
	}
	providerID = modelregistry.MigrateProviderCode(strings.TrimSpace(providerID))
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

func (u *ModelRegistryUsecase) ProviderLogoURL(providerID string) string {
	providerID = modelregistry.MigrateProviderCode(strings.TrimSpace(providerID))
	return modelregistry.ProviderLogoURL(providerID)
}

func (u *ModelRegistryUsecase) HasProviderLogo(ctx context.Context, providerID string) bool {
	st, err := u.store(ctx)
	if err != nil {
		return false
	}
	providerID = modelregistry.MigrateProviderCode(strings.TrimSpace(providerID))
	return st.HasProviderLogo(providerID)
}

type systemSettingRootAdapter struct {
	repo SystemSettingRepo
}

func NewSystemSettingRootAdapter(repo SystemSettingRepo) ModelRegistryRootResolver {
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

func SeedModelRegistryCronTask(ctx context.Context, cronRepo CronRepo) error {
	tasks, err := cronRepo.ListCronTasks(ctx)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.TaskKey == "model-registry-sync" {
			return nil
		}
	}
	_, err = cronRepo.CreateCronTask(ctx, CronTask{
		TaskKey:    "model-registry-sync",
		Name:       "Model Registry Sync",
		Enabled:    true,
		Status:     "active",
		ConfigJSON: `{"target_type":"model_registry_sync","message":"sync model registry","interval_seconds":3600}`,
	})
	return err
}
