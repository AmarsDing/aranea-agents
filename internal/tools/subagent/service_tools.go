package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"

	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsubagent "trpc.group/trpc-go/trpc-agent-go/openclaw/subagent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type spawnTool struct {
	svc *Service
}

// LongRunning marks subagents_spawn as a long-running (asynchronous) tool.
// The tool returns immediately with a queued Run, while the actual sub-agent
// executes in the background. This flag is propagated to the frontend via
// EnvelopeToolCall.is_long_running so the UI can display an appropriate
// "running in background" indicator instead of a spinner.
func (t *spawnTool) LongRunning() bool { return true }

type listTool struct {
	svc *Service
}

type getTool struct {
	svc *Service
}

type cancelTool struct {
	svc *Service
}

type spawnInput struct {
	Task           string `json:"task"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Kind           string `json:"kind"`
	SubagentType   string `json:"subagent_type"`
}

type getInput struct {
	ID           string `json:"id"`
	BlockUntilMS int    `json:"block_until_ms"`
	// FullResult 是上行压缩的逃生舱（C4-③）：true 时返回存储全量
	// result（≤ResultRunes），默认 false 只回 ≤UpwardPipeMaxRunes 的
	// 截断视图——防轮询路径把完整交付物反复回灌父上下文（S07 570K）。
	FullResult bool `json:"full_result"`
}

type runIDInput struct {
	ID string `json:"id"`
}

type listResult struct {
	Runs []map[string]any `json:"runs,omitempty"`
}

func newSpawnTool(svc *Service) *spawnTool {
	return &spawnTool{svc: svc}
}

func newListTool(svc *Service) *listTool {
	return &listTool{svc: svc}
}

func newGetTool(svc *Service) *getTool {
	return &getTool{svc: svc}
}

func newCancelTool(svc *Service) *cancelTool {
	return &cancelTool{svc: svc}
}

func (t *spawnTool) Declaration() *trpctool.Declaration {

	limit := defaultMaxConcurrentSubAgents
	if t != nil && t.svc != nil && t.svc.maxConcurrent > 0 {
		limit = t.svc.maxConcurrent
	}
	return &trpctool.Declaration{
		Name: toolSubagentsSpawn,
		Description: fmt.Sprintf("Spawn one background subagent for the current "+
			"session. Use kind=explore for codebase search (avoid writes) or "+
			"kind=verify for tests/builds. Use this for long-running work, "+
			"parallelizable work, or independent verification. It returns "+
			"immediately with a run id. At most %d subagent runs may be "+
			"active concurrently; when the limit is reached the call fails "+
			"with a 429 — do NOT retry immediately; track the active runs "+
			"with subagents_get (optional block_until_ms) and wait for their "+
			"completion notifications before spawning more.", limit),
		InputSchema: &trpctool.Schema{
			Type:     schemaTypeObject,
			Required: []string{argTask},
			Properties: map[string]*trpctool.Schema{
				argTask: {
					Type:        schemaTypeString,
					Description: "Delegated task for the background subagent.",
				},
				argTimeoutSeconds: {
					Type:        schemaTypeInteger,
					Description: "Optional timeout in seconds for the delegated run.",
				},
				argKind: {
					Type:        schemaTypeString,
					Description: "Subagent kind: explore (search/read, avoid writes), verify (tests/builds), or general. Alias: subagent_type.",
				},
				argSubagentType: {
					Type:        schemaTypeString,
					Description: "Alias for kind.",
				},
			},
		},
	}
}

// Call executes the subagents_spawn tool. It returns the queued Run object
// (not nil) so that the LLM receives a tool_result confirming the spawn
// succeeded and containing the Run ID for subsequent subagents_get queries.
// Returning nil would cause the framework to skip tool_result generation,
// leaving an orphaned tool_call in the LLM conversation and violating the
// tool_call/tool_result pairing required by LLM APIs.
func (t *spawnTool) Call(ctx context.Context, args []byte) (any, error) {
	if t == nil || t.svc == nil {
		return nil, apierror.Internal(apierror.DomainSubagent, "service unavailable")
	}
	if isNestedSubagent(ctx) {
		return nil, apierror.BadRequest(apierror.DomainSubagent, "nested subagent spawn is not supported")
	}

	var in spawnInput
	if err := json.Unmarshal(args, &in); err != nil {
		t.svc.lg.Warn("failed to unmarshal spawn arguments",
			loggateway.StepID("tool.subagent.spawn"),
			loggateway.Err(err),
		)
		return nil, apierror.BadRequest(apierror.DomainSubagent, "invalid arguments: "+err.Error())
	}

	userID, sess, err := currentContext(ctx)
	if err != nil {
		return nil, err
	}

	resultRunes, summaryRunes := t.svc.getSessionRunes(sess.ID)

	run, err := t.svc.Spawn(ctx, SpawnRequest{
		OwnerUserID:     userID,
		ParentSessionID: sess.ID,
		Task:            in.Task,
		TimeoutSeconds:  in.TimeoutSeconds,
		Kind:            firstNonEmpty(in.Kind, in.SubagentType),
		ResultRunes:     resultRunes,
		SummaryRunes:    summaryRunes,
	})
	if err != nil {
		return nil, err
	}
	return enrichRunView(run, svcNow(t.svc)), nil
}

func (t *listTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: toolSubagentsList,
		Description: "List background subagents created from the " +
			"current session. Returns status and short summary per run " +
			"(results are omitted — fetch one with subagents_get).",
		InputSchema: &trpctool.Schema{
			Type: schemaTypeObject,
		},
	}
}

func (t *listTool) Call(ctx context.Context, args []byte) (any, error) {
	if t == nil || t.svc == nil {
		return nil, apierror.Internal(apierror.DomainSubagent, "service unavailable")
	}
	if len(args) > 0 && strings.TrimSpace(string(args)) != "" &&
		strings.TrimSpace(string(args)) != "{}" {
		var ignored map[string]any
		if err := json.Unmarshal(args, &ignored); err != nil {
			t.svc.lg.Warn("failed to unmarshal list arguments",
				loggateway.StepID("tool.subagent.list"),
				loggateway.Err(err),
			)
			return nil, apierror.BadRequest(apierror.DomainSubagent, "invalid arguments: "+err.Error())
		}
	}

	userID, sess, err := currentContext(ctx)
	if err != nil {
		return nil, err
	}
	now := svcNow(t.svc)
	raw := t.svc.ListForUser(
		userID,
		trpcsubagent.ListFilter{
			ParentSessionID: sess.ID,
		},
	)
	views := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		view := enrichRunView(r, now)

		delete(view, "result")
		views = append(views, view)
	}
	return listResult{Runs: views}, nil
}

func (t *getTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: toolSubagentsGet,
		Description: "Get the latest status and result for one " +
			"background subagent run. Optional block_until_ms waits until the run is terminal. " +
			"The result is truncated to 2000 runes by default; pass full_result=true once " +
			"if you genuinely need the complete stored result.",
		InputSchema: &trpctool.Schema{
			Type:     schemaTypeObject,
			Required: []string{argID},
			Properties: map[string]*trpctool.Schema{
				argID: {
					Type:        schemaTypeString,
					Description: "Subagent run id returned by spawn.",
				},
				argBlockUntilMS: {
					Type:        schemaTypeInteger,
					Description: "Wait this many milliseconds for a terminal status before returning.",
				},
				argFullResult: {
					Type:        schemaTypeBoolean,
					Description: "Return the complete stored result instead of the default 2000-rune truncated view.",
				},
			},
		},
	}
}

func (t *getTool) Call(ctx context.Context, args []byte) (any, error) {
	if t == nil || t.svc == nil {
		return nil, apierror.Internal(apierror.DomainSubagent, "service unavailable")
	}
	var in getInput
	if err := json.Unmarshal(args, &in); err != nil {
		t.svc.lg.Warn("failed to unmarshal get arguments",
			loggateway.StepID("tool.subagent.get"),
			loggateway.Err(err),
		)
		return nil, apierror.BadRequest(apierror.DomainSubagent, "invalid arguments: "+err.Error())
	}
	userID, _, err := currentContext(ctx)
	if err != nil {
		return nil, err
	}
	runID := strings.TrimSpace(in.ID)
	if runID == "" {
		return nil, apierror.BadRequest(apierror.DomainSubagent, "empty run id")
	}
	wait := time.Duration(0)
	if in.BlockUntilMS > 0 {
		wait = time.Duration(in.BlockUntilMS) * time.Millisecond
	}
	run, err := t.svc.WaitForUser(ctx, userID, runID, wait)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, trpcsubagent.ErrRunNotFound
	}
	view := enrichRunView(*run, svcNow(t.svc))

	if !in.FullResult {
		clipUpwardResultView(view)
	}
	return view, nil
}

func (t *cancelTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: toolSubagentsCancel,
		Description: "Cancel one background subagent run. This is " +
			"best-effort.",
		InputSchema: &trpctool.Schema{
			Type:     schemaTypeObject,
			Required: []string{argID},
			Properties: map[string]*trpctool.Schema{
				argID: {
					Type:        schemaTypeString,
					Description: "Subagent run id returned by spawn.",
				},
			},
		},
	}
}

func (t *cancelTool) Call(ctx context.Context, args []byte) (any, error) {
	if t == nil || t.svc == nil {
		return nil, apierror.Internal(apierror.DomainSubagent, "service unavailable")
	}
	runID, userID, err := decodeRunIDArgs(ctx, args, t.svc.lg)
	if err != nil {
		return nil, err
	}
	run, _, err := t.svc.CancelForUser(userID, runID)
	if err != nil {
		return nil, err
	}
	return run, nil
}

func decodeRunIDArgs(ctx context.Context, args []byte, lg loggateway.Logger) (string, string, error) {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	var in runIDInput
	if err := json.Unmarshal(args, &in); err != nil {
		lg.Warn("failed to unmarshal run_id arguments",
			loggateway.StepID("tool.subagent.decode_run_id"),
			loggateway.Err(err),
		)
		return "", "", apierror.BadRequest(apierror.DomainSubagent, "invalid arguments: "+err.Error())
	}
	userID, _, err := currentContext(ctx)
	if err != nil {
		return "", "", err
	}
	runID := strings.TrimSpace(in.ID)
	if runID == "" {
		return "", "", apierror.BadRequest(apierror.DomainSubagent, "empty run id")
	}
	return runID, userID, nil
}

func currentContext(ctx context.Context) (string, *trpcsession.Session, error) {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return "", nil, apierror.BadRequest(apierror.DomainSubagent, "current session context is unavailable")
	}
	userID := strings.TrimSpace(inv.Session.UserID)
	if userID == "" {
		return "", nil, apierror.BadRequest(apierror.DomainSubagent, "current user id is unavailable")
	}
	if strings.TrimSpace(inv.Session.ID) == "" {
		return "", nil, apierror.BadRequest(apierror.DomainSubagent, "current session id is unavailable")
	}
	return userID, inv.Session, nil
}

func isNestedSubagent(ctx context.Context) bool {
	nested, ok := trpcagent.GetRuntimeStateValueFromContext[bool](
		ctx,
		runtimeStateSubagentRun,
	)
	return ok && nested
}
