package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"aranea-agents/internal/modelregistry"
	"aranea-agents/internal/tools/modelsync"
	"aranea-agents/pkg/safego"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type ModelRegistrySyncAgent struct {
	phases    []modelregistry.Phase
	logoPhase modelregistry.Phase
	tools     []trpctool.Tool
	storeProv modelregistry.StoreProvider
	backend   modelregistry.ApplyBackend
	runner    trpcrunner.Runner
}

func BuildModelRegistrySyncAgent(
	storeProv modelregistry.StoreProvider,
	backend modelregistry.ApplyBackend,
) (*ModelRegistrySyncAgent, error) {
	phases := modelsync.BuildPhases(backend)
	tools := modelsync.RegisterAll(modelsync.Deps{
		Phases:        phases,
		StoreProvider: storeProv,
		Backend:       backend,
	})

	ag := &ModelRegistrySyncAgent{
		phases:    phases.List(),
		logoPhase: phases.LogoPhase(),
		tools:     tools,
		storeProv: storeProv,
		backend:   backend,
	}

	ag.runner = trpcrunner.NewRunner("model-registry-sync", ag)

	return ag, nil
}

func (a *ModelRegistrySyncAgent) Run(ctx context.Context, inv *trpcagent.Invocation) (<-chan *trpcevent.Event, error) {
	ch := make(chan *trpcevent.Event, 64)
	invocationID := resolveInvocationID(inv)
	author := "model-registry-sync"

	safego.Go(ctx, "modelregistry.sync_agent", func() {
		defer close(ch)

		store, err := a.storeProv.Store(ctx)
		if err != nil {
			emitSyncError(ctx, inv, ch, invocationID, author, err)
			return
		}

		policy, policyErr := store.LoadPolicy()
		if policyErr != nil {
			emitSyncError(ctx, inv, ch, invocationID, author, fmt.Errorf("load policy: %w", policyErr))
			return
		}

		pc := &modelregistry.PhaseContext{
			Ctx:      ctx,
			Store:    store,
			Backend:  a.backend,
			Reader:   a.backend,
			Writer:   a.backend,
			Migrator: a.backend,
			Policy:   policy,
		}

		for _, phase := range a.phases {
			phaseCtx, cancel := context.WithTimeout(ctx, phase.Timeout())
			pc.Ctx = phaseCtx

			emitPhaseStart(ctx, inv, ch, invocationID, author, phase.Name())
			start := time.Now()
			result := phase.Run(pc)
			result.Duration = time.Since(start)
			cancel()
			emitPhaseResult(ctx, inv, ch, invocationID, author, phase.Name(), result)

			if result.Status == modelregistry.PhaseFailed {
				return
			}

			if phase.Name() == "fetch" && result.Status == modelregistry.PhaseSucceeded {
				dir, _, dirErr := store.LoadDirectory()
				if dirErr != nil {
					emitSyncError(ctx, inv, ch, invocationID, author, fmt.Errorf("load directory after fetch: %w", dirErr))
					return
				}
				pc.Directory = dir
			}
		}

		if pc.Directory != nil && len(pc.Directory) > 0 {
			logoCtx, logoCancel := context.WithTimeout(ctx, a.logoPhase.Timeout())
			pc.Ctx = logoCtx
			emitPhaseStart(ctx, inv, ch, invocationID, author, a.logoPhase.Name())
			start := time.Now()
			logoResult := a.logoPhase.Run(pc)
			logoResult.Duration = time.Since(start)
			logoCancel()
			emitPhaseResult(ctx, inv, ch, invocationID, author, a.logoPhase.Name(), logoResult)
		}
	})

	return ch, nil
}

func (a *ModelRegistrySyncAgent) Tools() []trpctool.Tool { return a.tools }
func (a *ModelRegistrySyncAgent) Info() trpcagent.Info {
	return trpcagent.Info{Name: "model-registry-sync", Description: "Model registry sync agent (programmatic)"}
}
func (a *ModelRegistrySyncAgent) SubAgents() []trpcagent.Agent        { return nil }
func (a *ModelRegistrySyncAgent) FindSubAgent(string) trpcagent.Agent { return nil }

func (a *ModelRegistrySyncAgent) RunSync(ctx context.Context) error {
	ch, err := a.runner.Run(ctx, "system", "model-registry-sync", trpcmodel.NewUserMessage("run model registry sync"))
	if err != nil {
		return err
	}
	for range ch {
	}
	return nil
}

func resolveInvocationID(inv *trpcagent.Invocation) string {
	if inv != nil && inv.InvocationID != "" {
		return inv.InvocationID
	}
	return "model-registry-sync"
}

func emitPhaseStart(ctx context.Context, inv *trpcagent.Invocation, ch chan<- *trpcevent.Event, invocationID, author, phase string) {
	evt := trpcevent.New(invocationID, author,
		trpcevent.WithTag("phase_start"),
		trpcevent.WithExtension("phase", phase),
	)
	trpcagent.EmitEvent(ctx, inv, ch, evt)
}

func emitPhaseResult(ctx context.Context, inv *trpcagent.Invocation, ch chan<- *trpcevent.Event, invocationID, author, phase string, result modelregistry.PhaseResult) {
	evt := trpcevent.New(invocationID, author,
		trpcevent.WithTag("phase_"+string(result.Status)),
		trpcevent.WithExtension("phase", phase),
		trpcevent.WithExtension("status", string(result.Status)),
		trpcevent.WithExtension("duration_ms", result.Duration.Milliseconds()),
	)
	if len(result.Errors) > 0 {
		evt.Extensions["errors"], _ = json.Marshal(result.Errors)
	}
	trpcagent.EmitEvent(ctx, inv, ch, evt)
}

func emitSyncError(ctx context.Context, inv *trpcagent.Invocation, ch chan<- *trpcevent.Event, invocationID, author string, err error) {
	evt := trpcevent.NewErrorEvent(invocationID, author, "sync_error", err.Error())
	trpcagent.EmitEvent(ctx, inv, ch, evt)
}
