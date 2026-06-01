package modelsync

import (
	"context"
	"fmt"

	"aranea-agents/internal/modelregistry"

	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type noArgs struct{}

type fetchDirectoryOutput struct {
	Status  string   `json:"status"`
	Errors  []string `json:"errors,omitempty"`
	Message string   `json:"message,omitempty"`
}

type migrateProvidersOutput struct {
	Status         string   `json:"status"`
	CompletedRules []string `json:"completed_rules,omitempty"`
	SkippedRules   []string `json:"skipped_rules,omitempty"`
	Errors         []string `json:"errors,omitempty"`
	Message        string   `json:"message,omitempty"`
}

type applyDirectoryOutput struct {
	Status       string   `json:"status"`
	AppliedCount int      `json:"applied_count,omitempty"`
	Errors       []string `json:"errors,omitempty"`
	Message      string   `json:"message,omitempty"`
}

type syncProviderLogosOutput struct {
	Status      string   `json:"status"`
	SyncedCount int      `json:"synced_count,omitempty"`
	Errors      []string `json:"errors,omitempty"`
	Message     string   `json:"message,omitempty"`
}

func newFetchDirectoryTool(deps Deps) *trpcfunction.FunctionTool[noArgs, fetchDirectoryOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, _ noArgs) (fetchDirectoryOutput, error) {
			store, err := deps.StoreProvider.Store(ctx)
			if err != nil {
				return fetchDirectoryOutput{}, fmt.Errorf("store error: %w", err)
			}
			policy, policyErr := store.LoadPolicy()
			if policyErr != nil {
				return fetchDirectoryOutput{}, fmt.Errorf("load policy: %w", policyErr)
			}
			pc := &modelregistry.PhaseContext{
				Ctx:     ctx,
				Store:   store,
				Backend: deps.Backend,
				Reader:  deps.Backend,
				Writer:  deps.Backend,
				Policy:  policy,
			}
			result := deps.Phases.fetchPhase.Run(pc)
			if result.Status == modelregistry.PhaseFailed {
				return fetchDirectoryOutput{Status: "failed", Errors: result.Errors}, nil
			}
			return fetchDirectoryOutput{Status: "succeeded", Message: "model directory fetched"}, nil
		},
		trpcfunction.WithName("fetch_model_directory"),
		trpcfunction.WithDescription("Fetch the latest model directory from models.dev"),
	)
}

func newMigrateProvidersTool(deps Deps) *trpcfunction.FunctionTool[noArgs, migrateProvidersOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, _ noArgs) (migrateProvidersOutput, error) {
			store, err := deps.StoreProvider.Store(ctx)
			if err != nil {
				return migrateProvidersOutput{}, fmt.Errorf("store error: %w", err)
			}
			policy, policyErr := store.LoadPolicy()
			if policyErr != nil {
				return migrateProvidersOutput{}, fmt.Errorf("load policy: %w", policyErr)
			}
			pc := &modelregistry.PhaseContext{
				Ctx:      ctx,
				Store:    store,
				Backend:  deps.Backend,
				Migrator: deps.Backend,
				Policy:   policy,
			}
			result := deps.Phases.migratePhase.Run(pc)
			if result.Status == modelregistry.PhaseFailed {
				return migrateProvidersOutput{Status: "failed", Errors: result.Errors}, nil
			}
			return migrateProvidersOutput{Status: "succeeded", Message: "provider bindings migrated"}, nil
		},
		trpcfunction.WithName("migrate_provider_bindings"),
		trpcfunction.WithDescription("Migrate legacy provider bindings to current provider IDs"),
	)
}

func newApplyDirectoryTool(deps Deps) *trpcfunction.FunctionTool[noArgs, applyDirectoryOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, _ noArgs) (applyDirectoryOutput, error) {
			store, err := deps.StoreProvider.Store(ctx)
			if err != nil {
				return applyDirectoryOutput{}, fmt.Errorf("store error: %w", err)
			}
			policy, policyErr := store.LoadPolicy()
			if policyErr != nil {
				return applyDirectoryOutput{}, fmt.Errorf("load policy: %w", policyErr)
			}
			dir, _, dirErr := store.LoadDirectory()
			if dirErr != nil {
				return applyDirectoryOutput{}, fmt.Errorf("load directory: %w", dirErr)
			}
			pc := &modelregistry.PhaseContext{
				Ctx:       ctx,
				Store:     store,
				Backend:   deps.Backend,
				Reader:    deps.Backend,
				Writer:    deps.Backend,
				Directory: dir,
				Policy:    policy,
			}
			result := deps.Phases.applyPhase.Run(pc)
			if result.Status == modelregistry.PhaseFailed {
				return applyDirectoryOutput{Status: "failed", Errors: result.Errors}, nil
			}
			return applyDirectoryOutput{Status: "succeeded", Message: "model directory applied"}, nil
		},
		trpcfunction.WithName("apply_model_directory"),
		trpcfunction.WithDescription("Apply model directory changes to database"),
	)
}

func newSyncProviderLogosTool(deps Deps) *trpcfunction.FunctionTool[noArgs, syncProviderLogosOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, _ noArgs) (syncProviderLogosOutput, error) {
			store, err := deps.StoreProvider.Store(ctx)
			if err != nil {
				return syncProviderLogosOutput{}, fmt.Errorf("store error: %w", err)
			}
			policy, policyErr := store.LoadPolicy()
			if policyErr != nil {
				return syncProviderLogosOutput{}, fmt.Errorf("load policy: %w", policyErr)
			}
			dir, _, dirErr := store.LoadDirectory()
			if dirErr != nil {
				return syncProviderLogosOutput{}, fmt.Errorf("load directory: %w", dirErr)
			}
			pc := &modelregistry.PhaseContext{
				Ctx:       ctx,
				Store:     store,
				Backend:   deps.Backend,
				Directory: dir,
				Policy:    policy,
			}
			result := deps.Phases.LogoPhase().Run(pc)
			if result.Status == modelregistry.PhaseFailed {
				return syncProviderLogosOutput{Status: "failed", Errors: result.Errors}, nil
			}
			return syncProviderLogosOutput{Status: "succeeded", Message: "provider logos synced"}, nil
		},
		trpcfunction.WithName("sync_provider_logos"),
		trpcfunction.WithDescription("Download and cache provider logos from models.dev"),
	)
}
