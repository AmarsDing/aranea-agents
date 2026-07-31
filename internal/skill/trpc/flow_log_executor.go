package trpc

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

// WrapWithFlowLog wraps a CodeExecutor so skill_run program executions emit
// skill.execute flow logs (done/error only — skill_run may fire multiple
// times within a single Turn, so no start phase is emitted) plus a
// process-level Error log on failure.
//
// Gate neutrality: llmagent derives tool-surface decisions (workspace_exec
// visibility, live workspace facade installation, interactive session
// support) from whether the agent executor implements
// codeexecutor.EngineProvider. Adding Engine() to a chain that previously
// did not expose it would silently enable the workspace_exec tool surface.
// This wrapper therefore only forwards Engine() when the wrapped executor
// already implements EngineProvider; otherwise it is a pure pass-through
// and emission stays dormant. Today the production chain (metrics +
// artifact wrappers) does not forward EngineProvider, so activating
// skill.execute flow logs additionally requires the wrapper chain to
// forward Engine() — a deliberate cross-domain decision, since a forwarded
// engine also flips the llmagent workspace_exec surface gates.
func WrapWithFlowLog(inner codeexecutor.CodeExecutor, bus contract.MonitorBus, lg loggateway.Logger) codeexecutor.CodeExecutor {
	if inner == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	if ep, ok := inner.(codeexecutor.EngineProvider); ok {
		return &flowLoggingEPExecutor{
			flowLoggingExecutor: flowLoggingExecutor{inner: inner, bus: bus, lg: lg},
			ep:                  ep,
		}
	}
	return &flowLoggingExecutor{inner: inner, bus: bus, lg: lg}
}

// flowLoggingExecutor is the pass-through variant used when the wrapped
// chain does not expose an engine: it deliberately has no Engine() method
// so llmagent surface gates observe the same executor shape as before
// wrapping. bus/lg are carried for the EngineProvider variant.
type flowLoggingExecutor struct {
	inner codeexecutor.CodeExecutor
	bus   contract.MonitorBus
	lg    loggateway.Logger
}

func (w *flowLoggingExecutor) CodeBlockDelimiter() codeexecutor.CodeBlockDelimiter {
	return w.inner.CodeBlockDelimiter()
}

func (w *flowLoggingExecutor) ExecuteCode(ctx context.Context, input codeexecutor.CodeExecutionInput) (codeexecutor.CodeExecutionResult, error) {
	return w.inner.ExecuteCode(ctx, input)
}

// flowLoggingEPExecutor forwards Engine() with a flow-logging runner. It is
// selected only when the inner executor is already an EngineProvider, so
// surface gates resolve identically to the unwrapped executor.
type flowLoggingEPExecutor struct {
	flowLoggingExecutor
	ep codeexecutor.EngineProvider

	engineOnce sync.Once
	engine     codeexecutor.Engine
}

// Engine implements codeexecutor.EngineProvider. A nil inner engine is
// preserved as nil so gates that nil-check the engine behave as before.
func (w *flowLoggingEPExecutor) Engine() codeexecutor.Engine {
	w.engineOnce.Do(func() {
		if inner := w.ep.Engine(); inner != nil {
			w.engine = &flowLoggingEngine{inner: inner, bus: w.bus, lg: w.lg}
		}
	})
	return w.engine
}

type flowLoggingEngine struct {
	inner codeexecutor.Engine
	bus   contract.MonitorBus
	lg    loggateway.Logger
}

func (e *flowLoggingEngine) Manager() codeexecutor.WorkspaceManager { return e.inner.Manager() }
func (e *flowLoggingEngine) FS() codeexecutor.WorkspaceFS           { return e.inner.FS() }
func (e *flowLoggingEngine) Describe() codeexecutor.Capabilities    { return e.inner.Describe() }

// Runner preserves nil-runner and InteractiveProgramRunner semantics so the
// llmagent gates (workspace_exec sessions, interactive support) resolve
// exactly as they would for the unwrapped engine.
func (e *flowLoggingEngine) Runner() codeexecutor.ProgramRunner {
	r := e.inner.Runner()
	if r == nil {
		return nil
	}
	fr := flowLoggingRunner{inner: r, bus: e.bus, lg: e.lg}
	if ir, ok := r.(codeexecutor.InteractiveProgramRunner); ok {
		return &flowLoggingInteractiveRunner{flowLoggingRunner: fr, interactive: ir}
	}
	return &fr
}

// flowLoggingInteractiveRunner forwards StartProgram so interactive session
// capability detection (InteractiveProgramRunner) is preserved.
type flowLoggingInteractiveRunner struct {
	flowLoggingRunner
	interactive codeexecutor.InteractiveProgramRunner
}

func (r *flowLoggingInteractiveRunner) StartProgram(
	ctx context.Context,
	ws codeexecutor.Workspace,
	spec codeexecutor.InteractiveProgramSpec,
) (codeexecutor.ProgramSession, error) {
	return r.interactive.StartProgram(ctx, ws, spec)
}

type flowLoggingRunner struct {
	inner codeexecutor.ProgramRunner
	bus   contract.MonitorBus
	lg    loggateway.Logger
}

func (r *flowLoggingRunner) RunProgram(ctx context.Context, ws codeexecutor.Workspace, spec codeexecutor.RunProgramSpec) (codeexecutor.RunResult, error) {
	res, err := r.inner.RunProgram(ctx, ws, spec)
	r.emit(ctx, spec, res, err)
	return res, err
}

// emit reports one skill_run execution. Non-skill program runs (no
// SKILL_NAME env, e.g. workspace_exec through the same engine) are skipped.
func (r *flowLoggingRunner) emit(ctx context.Context, spec codeexecutor.RunProgramSpec, res codeexecutor.RunResult, runErr error) {
	slug := strings.TrimSpace(spec.Env[codeexecutor.EnvSkillName])
	if slug == "" {
		return
	}
	cmd := truncateCmd(spec.Cmd + " " + strings.Join(spec.Args, " "))
	failed := runErr != nil || res.ExitCode != 0 || res.TimedOut
	errMsg := ""
	switch {
	case runErr != nil:
		errMsg = runErr.Error()
	case res.TimedOut:
		errMsg = "execution timed out"
	case res.ExitCode != 0:
		errMsg = "exit code " + strconv.Itoa(res.ExitCode)
	}
	if failed {
		r.lg.Error("Skill 运行时执行失败",
			loggateway.StepID("skill.execute"),
			loggateway.Str("slug", slug),
			loggateway.Str("cmd", cmd),
			loggateway.Int("exit_code", res.ExitCode),
			loggateway.Str("error", errMsg))
	}
	if r.bus == nil {
		return
	}
	flow := event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:       ctx,
		SessionID: sessionIDFromContext(ctx),
		Domain:    event.TraceDomainSkill,
		LG:        r.lg,
		Infra:     event.NewInfraFromBus(r.bus),
	})
	pairs := []event.Pair{
		event.P("slug", slug),
		event.P("cmd", cmd),
		event.P("exit_code", res.ExitCode),
		event.P("timed_out", res.TimedOut),
		event.P("duration_ms", res.Duration.Milliseconds()),
	}
	if failed {
		flow.LogError("skill.execute", "Skill 运行时执行失败",
			append(pairs, event.P("error", errMsg))...)
		return
	}
	flow.LogDone("skill.execute", "Skill 运行时执行完成", pairs...)
}

func sessionIDFromContext(ctx context.Context) string {
	if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil && inv.Session != nil {
		return inv.Session.ID
	}
	return ""
}

// truncateCmd caps command previews so flow-log extras never carry
// full-length command lines (which may embed credentials).
func truncateCmd(s string) string {
	const maxRunes = 200
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "..."
}
