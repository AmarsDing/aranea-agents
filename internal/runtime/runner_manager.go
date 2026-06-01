package runtime

import (
	"strings"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

// RunnerFactoryDeps holds shared persistence services for runner construction.
type RunnerFactoryDeps struct {
	Persist PersistenceSet
}

// TurnRunnerSpec configures a single-turn managed runner.
type TurnRunnerSpec struct {
	Plugins               []trpcplugin.Plugin
	AwaitUserReplyRouting bool
	BuilderDeps           chatagent.TRPCBuilderDeps
	AgentFactoryKeys      []string
	LookupAgents          map[string]trpcagent.Agent
	RalphLoop             *trpcrunner.RalphLoopConfig
	ExtraOpts             []trpcrunner.Option
	// RegistryKey, when set, stores the runner in the instance registry until CloseRunner.
	RegistryKey string
}

// RunnerManager centralizes trpc runner assembly (session/memory/artifact/plugins/factories).
type RunnerManager struct {
	factory  RunnerFactoryDeps
	registry *RunnerInstanceRegistry
	lg       loggateway.Logger
}

func NewRunnerManager(factory RunnerFactoryDeps, lg loggateway.Logger) *RunnerManager {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &RunnerManager{
		factory:  factory,
		registry: NewRunnerInstanceRegistry(),
		lg:       lg,
	}
}

// Registry exposes the optional long-lived runner instance registry.
func (m *RunnerManager) Registry() *RunnerInstanceRegistry {
	if m == nil {
		return nil
	}
	return m.registry
}

// NewTurnRunner builds a ManagedRunner for one chat/team turn.
func (m *RunnerManager) NewTurnRunner(root trpcagent.Agent, spec TurnRunnerSpec) (trpcrunner.ManagedRunner, error) {
	if m == nil {
		return nil, errRunnerManagerNil
	}
	runnerDeps := chatagent.NewRunnerDepsFromRuntime(
		m.factory.Persist.Session,
		m.factory.Persist.Memory.TRPC,
		m.factory.Persist.Artifact,
		spec.Plugins...,
	)
	runnerDeps.AwaitUserReplyRouting = spec.AwaitUserReplyRouting
	runnerDeps.RalphLoop = spec.RalphLoop

	opts := append([]trpcrunner.Option{}, spec.ExtraOpts...)
	opts = append(opts, chatagent.BizAgentRegistryOptions(spec.LookupAgents)...)
	if len(spec.AgentFactoryKeys) > 0 {
		opts = append(opts, chatagent.BizAgentFactoryOptions(spec.BuilderDeps, spec.AgentFactoryKeys...)...)
	}

	mr, err := chatagent.NewTRPCRunner(root, runnerDeps, opts...)
	if err != nil {
		return nil, err
	}

	if key := strings.TrimSpace(spec.RegistryKey); key != "" {
		if prev, ok := m.registry.Replace(key, mr); ok && prev != nil {
			if err := prev.Close(); err != nil {
				m.lg.Warn("close prev runner", loggateway.StepID("runtime.runner_manager"), loggateway.Str("key", key), loggateway.Err(err))
			}
		}
	}
	return mr, nil
}

// CloseRunner closes and unregisters a long-lived runner by registry key.
func (m *RunnerManager) CloseRunner(key string) error {
	if m == nil {
		return errRunnerManagerNil
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	runner, ok := m.registry.Unregister(key)
	if !ok || runner == nil {
		return nil
	}
	return runner.Close()
}
