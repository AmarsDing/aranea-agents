# LLM Activity Ordering Phase 3b-D: Delete v1 ActivityEventBus + activities Table

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Delete the v1 `ActivityEventBus` (40 direct-publish callers), the v1 `activities` Ent table + repo, and the v1 WS `activity_event` subscription path — after migrating all reads to v2 entities (Step/Task/Turn/etc.) and all publishes to the v2 `EventBus`.

**Architecture:** The v2 event pipeline (V2Bus + Sequencer + WSV2Subscriber + frontend `v2_event` envelope) is already wired for chat-rendering events. Phase 3b-D extends it to cover: (1) history-read API (new v2 proto + service + frontend fetch), (2) direct-publish callers (40 sites migrated to `EventBus.Publish` with typed `biz.Event`), (3) system-domain events (run_status/heartbeat re-routed via v2 notice steps), then deletes all v1 code.

**Tech Stack:** Go, Kratos v2, trpc-agent-go, Wire DI, Ent ORM, Vue 3/TypeScript/Pinia

**Spec:** `docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md` §Phase 3

---

## File Structure

### Created Files

| File | Responsibility |
|------|---------------|
| `api/kratos/session/v1/session_v2.proto` | v2 entity read RPCs (ListTasksV2, ListTurnsV2, ListStepsV2, GetStepV2) |
| `internal/service/session_v2.go` | v2 service: implements session_v2 proto RPCs |
| `internal/biz/event_system.go` | v2 system-domain Event types (RunStatusEvent, HeartbeatEvent, etc.) |
| `web/src/features/session/v2Api.ts` | Frontend v2 API client wrappers |

### Modified Files

| File | Change |
|------|--------|
| `internal/biz/repo_ports_v2.go` | Add `ListStepsBySession` to `StepV2Reader` |
| `internal/data/step_v2_repo.go` | Implement `ListStepsBySession` |
| `internal/service/session.go` | `ListActivities` delegates to v2 reader |
| `internal/service/chat_confirm.go` | `ConfirmActivity` uses `StepV2Reader.GetStep` |
| `internal/service/team_dead_letter.go` | `ListSpiritTeams` uses `TeamStageV2Reader` |
| `internal/chatactivity/cancel.go` | `CancelRunningActivityMessages` uses v2 Step reader |
| `internal/biz/session_activity_adapter.go` | Adapt to v2 Step reader |
| 40 files with `activityBus.Publish` | Migrate to `eventBus.Publish` with typed Event |
| `internal/server/ws_event.go` | Remove v1 activity subscription |
| `internal/server/ws_io_pump.go` | Remove `activityEventPump` |
| `internal/server/ws.go` | Remove activity pump goroutine |
| `cmd/admin/wire.go` | Remove v1 bus providers; add v2 service providers |
| `web/src/stores/chat/activityV2Store.ts` | Add `fetchSessionHistory` action |
| `web/src/features/chat/composables/useChatEventInspector.ts` | Use v2 fetch |
| `web/src/realtime/ws-transport.ts` | Remove `activity_event` parsing |
| `web/src/realtime/globalWsHub.ts` | Remove `onActivityEvent` callback |
| `web/src/features/chat/composables/useChatWorkspace.ts` | Remove v1 system event routing |

### Deleted Files

| File | Lines | Reason |
|------|-------|--------|
| `internal/biz/activity_event.go` | ~150 | v1 ActivityEvent + ActivityEventBus interface |
| `internal/biz/activity.go` | ~180 | v1 Activity model + Reader/Writer interfaces |
| `internal/data/activity_repo.go` | ~400 | v1 activity repo implementation |
| `internal/data/ent/schema/activity.go` | ~120 | v1 Ent schema |
| `internal/event/activityevent/bus.go` | ~150 | v1 bus implementation |
| `internal/biz/session_activity_adapter.go` | ~50 | v1 adapter (replaced) |
| `web/src/realtime/activityEvent.ts` | ~300 | v1 frontend types |
| `web/src/features/chat/composables/useSystemEventNotification.ts` | ~130 | v1 system event router |

---

## Tier 1: v2 History Read API (unblocks Task 2 deletion)

### Task 1: Add ListStepsBySession to StepV2Reader

**Files:**
- Modify: `internal/biz/repo_ports_v2.go:67-71`
- Modify: `internal/data/step_v2_repo.go`

- [ ] **Step 1: Add interface method**

In `internal/biz/repo_ports_v2.go`, add `ListStepsBySession` to `StepV2Reader`:

```go
type StepV2Reader interface {
	GetStep(ctx context.Context, id string) (Step, error)
	ListStepsByTurn(ctx context.Context, turnID string) ([]Step, error)
	ListStepsByTask(ctx context.Context, taskID string) ([]Step, error)
	ListStepsBySession(ctx context.Context, sessionID string) ([]Step, error) // NEW: replaces v1 ListBySession
}
```

- [ ] **Step 2: Implement in step_v2_repo.go**

In `internal/data/step_v2_repo.go`, add the method. Query via `spirit_session_id` index (StepV2 schema already has this index):

```go
func (r *stepV2Repo) ListStepsBySession(ctx context.Context, sessionID string) ([]biz.Step, error) {
	client := r.data.RW().Read(ctx)
	results, err := client.StepV2.Query().
		Where(stepv2.SpiritSessionID(sessionID)).
		Order(ent.Asc(stepv2.FieldStartedAt)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "step_v2")
	}
	out := make([]biz.Step, 0, len(results))
	for _, s := range results {
		out = append(out, entStepV2ToBiz(s))
	}
	return out, nil
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/data/... -run TestStepV2 -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/biz/repo_ports_v2.go internal/data/step_v2_repo.go
git commit -m "feat(data): add ListStepsBySession to StepV2Reader for v1 ListBySession replacement"
```

---

### Task 2: Add v2 read proto + service

**Files:**
- Create: `api/kratos/session/v1/session_v2.proto`
- Modify: `api/kratos/session/v1/session.proto` (add service reference)
- Create: `internal/service/session_v2.go`

- [ ] **Step 1: Write the v2 proto**

Create `api/kratos/session/v1/session_v2.proto`:

```protobuf
syntax = "proto3";

package session.v1;

option go_package = "aranea-agents/api/kratos/session/v1;sessionv1";

import "google/api/annotations.proto";

// v2 entity read RPCs — replaces v1 ListActivities.
service SessionV2Service {
  // ListTasks lists tasks for a session.
  rpc ListTasks(ListTasksV2Request) returns (ListTasksV2Response) {
    option (google.api.http) = {
      get: "/v2/sessions/{session_id}/tasks"
    };
  }

  // ListTurns lists turns for a task.
  rpc ListTurns(ListTurnsV2Request) returns (ListTurnsV2Response) {
    option (google.api.http) = {
      get: "/v2/tasks/{task_id}/turns"
    };
  }

  // ListSteps lists steps for a turn or session.
  rpc ListSteps(ListStepsV2Request) returns (ListStepsV2Response) {
    option (google.api.http) = {
      get: "/v2/sessions/{session_id}/steps"
    };
  }

  // GetStep returns a single step by ID (replaces v1 GetActivity for confirm).
  rpc GetStep(GetStepV2Request) returns (GetStepV2Response) {}
}

message ListTasksV2Request { string session_id = 1; }
message ListTasksV2Response { repeated TaskV2 tasks = 1; }

message ListTurnsV2Request { string task_id = 1; }
message ListTurnsV2Response { repeated TurnV2 turns = 1; }

message ListStepsV2Request {
  string session_id = 1;
  string turn_id = 2;   // optional: filter by turn
  string task_id = 3;   // optional: filter by task
}
message ListStepsV2Response { repeated StepV2 steps = 1; }

message GetStepV2Request { string step_id = 1; }
message GetStepV2Response { StepV2 step = 1; }

// Entity messages mirror biz structs (JSON tags match frontend v2Types.ts)
message TaskV2 {
  string id = 1;
  string session_id = 2;
  string spirit_session_id = 3;
  string user_message = 4;
  string status = 5;
  int64 version = 6;
  string started_at = 7;
  string completed_at = 8;
}

message TurnV2 {
  string id = 1;
  string task_id = 2;
  string session_id = 3;
  string agent_key = 4;
  string status = 5;
  int64 version = 6;
  int64 seq = 7;
  string started_at = 8;
  string completed_at = 9;
}

message StepV2 {
  string id = 1;
  string turn_id = 2;
  string task_id = 3;
  string session_id = 4;
  string spirit_session_id = 5;
  string kind = 6;
  string author_agent_key = 7;
  int64 seq = 8;
  int64 version = 9;
  string content = 10;
  string reasoning = 11;
  string tool_name = 12;
  string tool_call_id = 13;
  string tool_error_code = 14;
  string notice_type = 15;
  string status = 16;
  bool is_final = 17;
  string started_at = 18;
  string completed_at = 19;
  int64 tool_duration_ms = 20;
  bytes tool_args = 21;
  bytes tool_result = 22;
}
```

- [ ] **Step 2: Generate proto code**

Run: `make api`
Expected: generates `api/kratos/session/v1/session_v2.pb.go`

- [ ] **Step 3: Write failing service test**

Create `internal/service/session_v2_test.go`:

```go
package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	sessionv1 "aranea-agents/api/kratos/session/v1"
)

func TestSessionV2Service_ListSteps(t *testing.T) {
	// Stub StepV2Reader
	svc := &SessionV2Service{
		stepReader: &stubStepV2Reader{
			steps: []biz.Step{
				{ID: "s1", SessionID: "sess1", Kind: biz.StepKindReply, Content: "hello"},
			},
		},
	}
	resp, err := svc.ListSteps(context.Background(), &sessionv1.ListStepsV2Request{SessionId: "sess1"})
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if len(resp.Steps) != 1 || resp.Steps[0].Id != "s1" {
		t.Fatalf("unexpected: %+v", resp.Steps)
	}
}

type stubStepV2Reader struct {
	biz.StepV2Reader
	steps []biz.Step
}

func (s *stubStepV2Reader) ListStepsBySession(_ context.Context, _ string) ([]biz.Step, error) {
	return s.steps, nil
}
func (s *stubStepV2Reader) GetStep(_ context.Context, id string) (biz.Step, error) {
	for _, st := range s.steps {
		if st.ID == id {
			return st, nil
		}
	}
	return biz.Step{}, biz.ErrNotFound
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/service/ -run TestSessionV2Service -count=1`
Expected: FAIL (SessionV2Service not defined)

- [ ] **Step 5: Implement the service**

Create `internal/service/session_v2.go`:

```go
package service

import (
	"context"

	"aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"
)

// SessionV2Service implements session_v2 proto RPCs using v2 repo readers.
type SessionV2Service struct {
	sessionv1.UnimplementedSessionV2ServiceServer
	taskReader  biz.TaskV2Reader
	turnReader  biz.TurnV2Reader
	stepReader  biz.StepV2Reader
}

func NewSessionV2Service(taskReader biz.TaskV2Reader, turnReader biz.TurnV2Reader, stepReader biz.StepV2Reader) *SessionV2Service {
	return &SessionV2Service{taskReader: taskReader, turnReader: turnReader, stepReader: stepReader}
}

func (s *SessionV2Service) ListTasks(ctx context.Context, req *sessionv1.ListTasksV2Request) (*sessionv1.ListTasksV2Response, error) {
	tasks, err := s.taskReader.ListTasksBySession(ctx, req.SessionId)
	if err != nil {
		return nil, err
	}
	out := make([]*sessionv1.TaskV2, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, bizTaskToProto(t))
	}
	return &sessionv1.ListTasksV2Response{Tasks: out}, nil
}

func (s *SessionV2Service) ListTurns(ctx context.Context, req *sessionv1.ListTurnsV2Request) (*sessionv1.ListTurnsV2Response, error) {
	turns, err := s.turnReader.ListTurnsByTask(ctx, req.TaskId)
	if err != nil {
		return nil, err
	}
	out := make([]*sessionv1.TurnV2, 0, len(turns))
	for _, t := range turns {
		out = append(out, bizTurnToProto(t))
	}
	return &sessionv1.ListTurnsV2Response{Turns: out}, nil
}

func (s *SessionV2Service) ListSteps(ctx context.Context, req *sessionv1.ListStepsV2Request) (*sessionv1.ListStepsV2Response, error) {
	var steps []biz.Step
	var err error
	switch {
	case req.TurnId != "":
		steps, err = s.stepReader.ListStepsByTurn(ctx, req.TurnId)
	case req.TaskId != "":
		steps, err = s.stepReader.ListStepsByTask(ctx, req.TaskId)
	default:
		steps, err = s.stepReader.ListStepsBySession(ctx, req.SessionId)
	}
	if err != nil {
		return nil, err
	}
	out := make([]*sessionv1.StepV2, 0, len(steps))
	for _, st := range steps {
		out = append(out, bizStepToProto(st))
	}
	return &sessionv1.ListStepsV2Response{Steps: out}, nil
}

func (s *SessionV2Service) GetStep(ctx context.Context, req *sessionv1.GetStepV2Request) (*sessionv1.GetStepV2Response, error) {
	step, err := s.stepReader.GetStep(ctx, req.StepId)
	if err != nil {
		return nil, err
	}
	return &sessionv1.GetStepV2Response{Step: bizStepToProto(step)}, nil
}

// Conversion helpers (biz → proto)
func bizTaskToProto(t biz.Task) *sessionv1.TaskV2 {
	return &sessionv1.TaskV2{
		Id: t.ID, SessionId: t.SessionID, SpiritSessionId: t.SpiritSessionID,
		UserMessage: t.UserMessage, Status: string(t.Status),
		Version: t.Version,
	}
}

func bizTurnToProto(t biz.Turn) *sessionv1.TurnV2 {
	return &sessionv1.TurnV2{
		Id: t.ID, TaskId: t.TaskID, SessionId: t.SessionID,
		AgentKey: t.AgentKey, Status: string(t.Status),
		Version: t.Version, Seq: t.Seq,
	}
}

func bizStepToProto(s biz.Step) *sessionv1.StepV2 {
	return &sessionv1.StepV2{
		Id: s.ID, TurnId: s.TurnID, TaskId: s.TaskID, SessionId: s.SessionID,
		SpiritSessionId: s.SpiritSessionID, Kind: string(s.Kind),
		AuthorAgentKey: s.AuthorAgentKey, Seq: s.Seq, Version: s.Version,
		Content: s.Content, Reasoning: s.Reasoning, ToolName: s.ToolName,
		ToolCallId: s.ToolCallID, ToolErrorCode: s.ToolErrorCode,
		NoticeType: s.NoticeType, Status: string(s.Status), IsFinal: s.IsFinal,
		ToolDurationMs: s.ToolDurationMs,
	}
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/service/ -run TestSessionV2Service -count=1`
Expected: PASS

- [ ] **Step 7: Wire the service**

In `cmd/admin/wire.go`, add the provider:

```go
// In provider set:
sessionv2svc := service.NewSessionV2Service(taskV2Repo, turnV2Repo, stepV2Repo)
// Register HTTP handler for SessionV2Service
```

Run: `make wire && go build ./cmd/admin`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add api/kratos/session/v1/session_v2.proto internal/service/session_v2.go internal/service/session_v2_test.go cmd/admin/wire.go
git commit -m "feat(service): add v2 entity read RPCs (ListTasks/ListTurns/ListSteps/GetStep)"
```

---

### Task 3: Frontend v2 API client + store fetch

**Files:**
- Create: `web/src/features/session/v2Api.ts`
- Modify: `web/src/stores/chat/activityV2Store.ts`

- [ ] **Step 1: Create v2 API client**

Create `web/src/features/session/v2Api.ts`:

```typescript
// web/src/features/session/v2Api.ts
import { sessionApi } from './api';
import type { Task, Turn, Step } from '../chat/v2Types';

export async function listTasksV2(sessionId: string): Promise<Task[]> {
  const resp = await sessionApi.listTasksV2({ sessionId });
  return resp.tasks || [];
}

export async function listTurnsV2(taskId: string): Promise<Turn[]> {
  const resp = await sessionApi.listTurnsV2({ taskId });
  return resp.turns || [];
}

export async function listStepsV2(sessionId: string): Promise<Step[]> {
  const resp = await sessionApi.listStepsV2({ sessionId });
  return resp.steps || [];
}

export async function getStepV2(stepId: string): Promise<Step | null> {
  const resp = await sessionApi.getStepV2({ stepId });
  return resp.step || null;
}
```

- [ ] **Step 2: Add fetchSessionHistory to store**

In `web/src/stores/chat/activityV2Store.ts`, add a fetch action:

```typescript
// Add import at top:
import { listTasksV2, listTurnsV2, listStepsV2 } from '../../features/session/v2Api';

// Inside the store function, after upsert helpers, add:

// === History fetch (page refresh / WS reconnect) ===

async function fetchSessionHistory(sessionId: string) {
  const tasks = await listTasksV2(sessionId);
  for (const t of tasks) upsertTask(t);

  for (const t of tasks) {
    const turns = await listTurnsV2(t.ID);
    for (const turn of turns) upsertTurn(turn);

    const steps = await listStepsV2(sessionId);
    for (const s of steps) upsertStep(s);
  }
}
```

Return it from the store: add `fetchSessionHistory` to the returned object.

- [ ] **Step 3: Update useChatEventInspector to use v2 fetch**

In `web/src/features/chat/composables/useChatEventInspector.ts`, replace `listActivities` with `fetchSessionHistory`:

```typescript
// Replace:
// import { listActivities } from '../../session/api';
// const acts = await listActivities(id);

// With:
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
// ...
const store = useChatActivityStore();
await store.fetchSessionHistory(sessionId);
```

- [ ] **Step 4: Verify frontend build**

Run: `cd web && pnpm lint && pnpm build`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd web && git add src/features/session/v2Api.ts src/stores/chat/activityV2Store.ts src/features/chat/composables/useChatEventInspector.ts
git commit -m "feat(web): add v2 history fetch API + store fetchSessionHistory action"
```

---

## Tier 2: Migrate v1 readers to v2

### Task 4: Migrate SessionService.ListActivities

**Files:**
- Modify: `internal/service/session.go:608-634`

- [ ] **Step 1: Rewrite ListActivities to delegate to v2**

In `internal/service/session.go`, change `ListActivities` to call the v2 `SessionV2Service.ListSteps` instead of `ActivityReader`:

```go
func (s *SessionService) ListActivities(ctx context.Context, req *sessionv1.ListActivitiesRequest) (*sessionv1.ListActivitiesResponse, error) {
	// v2 path: delegate to SessionV2Service which reads from steps_v2 table.
	resp, err := s.sessionV2.ListSteps(ctx, &sessionv1.ListStepsV2Request{
		SessionId: req.SessionId,
		TurnId:    req.TurnId,
	})
	if err != nil {
		return nil, err
	}
	// Convert v2 steps to v1 Activity format for backward-compat response
	out := make([]*sessionv1.Activity, 0, len(resp.Steps))
	for _, st := range resp.Steps {
		out = append(out, stepV2ToActivityV1(st))
	}
	return &sessionv1.ListActivitiesResponse{Activities: out}, nil
}
```

Note: The v1 `ListActivitiesResponse` is kept temporarily for API backward compat. The frontend (Task 3) will switch to the v2 endpoint directly, after which the v1 response adapter can be removed.

- [ ] **Step 2: Run tests**

Run: `go test ./internal/service/ -run TestListActivities -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/service/session.go
git commit -m "refactor(service): ListActivities delegates to v2 StepV2Reader"
```

---

### Task 5: Migrate ChatService.ConfirmActivity

**Files:**
- Modify: `internal/service/chat_confirm.go:35`

- [ ] **Step 1: Replace GetActivity with GetStep**

In `internal/service/chat_confirm.go`, replace the `activityReader.GetActivity` call with `stepReader.GetStep`:

```go
// Replace:
// act, err := s.activityReader.GetActivity(ctx, req.ActivityId)

// With:
step, err := s.stepReader.GetStep(ctx, req.ActivityId)
```

Update subsequent field accesses from `act.*` to `step.*` (e.g., `act.Meta` → `step.Meta`, `act.Status` → `string(step.Status)`).

- [ ] **Step 2: Run tests**

Run: `go test ./internal/service/ -run TestConfirm -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/service/chat_confirm.go
git commit -m "refactor(service): ConfirmActivity uses StepV2Reader.GetStep"
```

---

### Task 6: Migrate TeamService.ListSpiritTeams

**Files:**
- Modify: `internal/service/team_dead_letter.go:110`

- [ ] **Step 1: Replace GetActivity with GetTeamStage**

In `internal/service/team_dead_letter.go`, replace `activityReader.GetActivity` with `teamStageReader.GetTeamStage`:

```go
// Replace:
// act, err := s.activityReader.GetActivity(ctx, agent.TeamStageActivityID(teams[i].ID))
// members := extractMembersFromMeta(act.Meta)

// With:
ts, err := s.teamStageReader.GetTeamStage(ctx, teams[i].ID)
// members now come from ts.Members (TeamStage struct field)
```

- [ ] **Step 2: Run tests**

Run: `go test ./internal/service/ -run TestListSpiritTeams -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/service/team_dead_letter.go
git commit -m "refactor(service): ListSpiritTeams uses TeamStageV2Reader"
```

---

### Task 7: Migrate CancelRunningActivityMessages + session_activity_adapter

**Files:**
- Modify: `internal/chatactivity/cancel.go:59`
- Modify: `internal/biz/session_activity_adapter.go`

- [ ] **Step 1: Replace ListBySession with ListStepsBySession**

In `internal/chatactivity/cancel.go`, replace `activityReader.ListBySession` with `stepReader.ListStepsBySession`:

```go
// Replace:
// acts, err := c.activityReader.ListBySession(ctx, sessionID)

// With:
steps, err := c.stepReader.ListStepsBySession(ctx, sessionID)
```

Update the cancel logic to filter by `step.Status == "running"` instead of `act.Status == "running"`.

- [ ] **Step 2: Update session_activity_adapter**

In `internal/biz/session_activity_adapter.go`, adapt `biz.ActivityReader` to `biz.StepV2Reader`:

```go
type sessionActivityLister struct {
	stepReader biz.StepV2Reader
}

func (a *sessionActivityLister) ListBySession(ctx context.Context, sessionID string) ([]biz.Activity, error) {
	steps, err := a.stepReader.ListStepsBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]biz.Activity, 0, len(steps))
	for _, s := range steps {
		out = append(out, stepToActivity(s))
	}
	return out, nil
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/chatactivity/... ./internal/biz/... -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/chatactivity/cancel.go internal/biz/session_activity_adapter.go
git commit -m "refactor(chatactivity): CancelRunningActivityMessages + adapter use StepV2Reader"
```

---

## Tier 3: Migrate v1 Publish to v2 EventBus

### Task 8: Create v2 system-domain Event types

**Files:**
- Create: `internal/biz/event_system.go`

The v1 system-domain events (run_status, heartbeat, knowledge_indexed) need v2 Event types since v2 EventBus only accepts `biz.Event`.

- [ ] **Step 1: Write the failing test**

Create `internal/biz/event_system_test.go`:

```go
package biz

import (
	"context"
	"testing"
	"time"
)

func TestRunStatusEvent(t *testing.T) {
	ev := NewRunStatusEvent("sess-1", "run-1", "running", map[string]any{"progress": 0.5})
	if ev.EventKind() != EventKindSystemRunStatus {
		t.Fatalf("kind: %s", ev.EventKind())
	}
	if ev.SpiritSessionID() != "sess-1" {
		t.Fatalf("session: %s", ev.SpiritSessionID())
	}
}

func TestHeartbeatEvent(t *testing.T) {
	ev := NewHeartbeatEvent("sess-1", "still working", time.Now())
	if ev.EventKind() != EventKindSystemHeartbeat {
		t.Fatalf("kind: %s", ev.EventKind())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/biz/ -run TestRunStatusEvent -count=1`
Expected: FAIL (undefined: NewRunStatusEvent)

- [ ] **Step 3: Implement system events**

Create `internal/biz/event_system.go`:

```go
package biz

import (
	"context"
	"time"
)

// System-domain EventKind values (not tied to a specific entity).
const (
	EventKindSystemRunStatus   EventKind = "system.run_status"
	EventKindSystemHeartbeat   EventKind = "system.heartbeat"
	EventKindSystemNotice      EventKind = "system.notice"
)

// RunStatusEvent signals a run status change (replaces v1 system-domain run_status ActivityEvent).
type RunStatusEvent struct {
	sessionID     string
	RunID         string
	Status        string
	Meta          map[string]any
	occurredAt    time.Time
}

func NewRunStatusEvent(sessionID, runID, status string, meta map[string]any) *RunStatusEvent {
	return &RunStatusEvent{
		sessionID:  sessionID,
		RunID:      runID,
		Status:     status,
		Meta:       meta,
		occurredAt: time.Now(),
	}
}

func (e *RunStatusEvent) EventKind() EventKind      { return EventKindSystemRunStatus }
func (e *RunStatusEvent) SpiritSessionID() string   { return e.sessionID }
func (e *RunStatusEvent) TaskID() string            { return "" }
func (e *RunStatusEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *RunStatusEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// HeartbeatEvent signals a run heartbeat (replaces v1 system-domain heartbeat ActivityEvent).
type HeartbeatEvent struct {
	sessionID  string
	Message    string
	occurredAt time.Time
}

func NewHeartbeatEvent(sessionID, message string, ts time.Time) *HeartbeatEvent {
	return &HeartbeatEvent{sessionID: sessionID, Message: message, occurredAt: ts}
}

func (e *HeartbeatEvent) EventKind() EventKind      { return EventKindSystemHeartbeat }
func (e *HeartbeatEvent) SpiritSessionID() string   { return e.sessionID }
func (e *HeartbeatEvent) TaskID() string            { return "" }
func (e *HeartbeatEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *HeartbeatEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// SystemNoticeEvent is a generic system notice (replaces v1 system-domain notice ActivityEvent).
type SystemNoticeEvent struct {
	sessionID  string
	NoticeType string
	Message    string
	Meta       map[string]any
	occurredAt time.Time
}

func NewSystemNoticeEvent(sessionID, noticeType, message string, meta map[string]any) *SystemNoticeEvent {
	return &SystemNoticeEvent{
		sessionID:  sessionID,
		NoticeType: noticeType,
		Message:    message,
		Meta:       meta,
		occurredAt: time.Now(),
	}
}

func (e *SystemNoticeEvent) EventKind() EventKind      { return EventKindSystemNotice }
func (e *SystemNoticeEvent) SpiritSessionID() string   { return e.sessionID }
func (e *SystemNoticeEvent) TaskID() string            { return "" }
func (e *SystemNoticeEvent) OccurredAt() time.Time     { return e.occurredAt }
func (e *SystemNoticeEvent) SetOccurredAt(t time.Time) { e.occurredAt = t }

// Ensure interface compliance
var (
	_ Event = (*RunStatusEvent)(nil)
	_ Event = (*HeartbeatEvent)(nil)
	_ Event = (*SystemNoticeEvent)(nil)
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/biz/ -run TestRunStatusEvent -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/biz/event_system.go internal/biz/event_system_test.go
git commit -m "feat(biz): add v2 system-domain Event types (RunStatus/Heartbeat/Notice)"
```

---

### Task 9: Migrate system-domain Publish (6 sites)

**Files:**
- Modify: `internal/service/run_status_publish.go`
- Modify: `internal/service/run_heartbeat.go`
- Modify: `internal/service/knowledge.go`

These 3 files contain 6 `activityBus.Publish` calls with `Domain: biz.ActivityDomainSystem`.

- [ ] **Step 1: Migrate run_status_publish.go**

In `internal/service/run_status_publish.go`, replace:

```go
// OLD:
s.activityBus.Publish(context.Background(), biz.ActivityEvent{
    Event:    biz.ActivityEventUpdated,
    Activity: biz.Activity{...},
    Domain:   biz.ActivityDomainSystem,
})

// NEW:
s.eventBus.Publish(context.Background(), biz.NewRunStatusEvent(sessionID, runID, status, meta))
```

Repeat for all `Publish` calls in this file (lines 142, 231 — 2 system-domain sites).

- [ ] **Step 2: Migrate run_heartbeat.go**

In `internal/service/run_heartbeat.go:99`, replace:

```go
// OLD:
r.activityBus.Publish(ctx, biz.ActivityEvent{
    Domain: biz.ActivityDomainSystem,
    Activity: biz.Activity{...},
})

// NEW:
r.eventBus.Publish(ctx, biz.NewHeartbeatEvent(sessionID, message, time.Now()))
```

- [ ] **Step 3: Migrate knowledge.go**

In `internal/service/knowledge.go:429`, replace:

```go
// OLD:
s.activityBus.Publish(ctx, biz.ActivityEvent{
    Domain: biz.ActivityDomainSystem,
    Activity: biz.Activity{...},
})

// NEW:
s.eventBus.Publish(ctx, biz.NewSystemNoticeEvent(sessionID, "knowledge_indexed", message, meta))
```

- [ ] **Step 4: Update struct fields**

In each modified file, replace `activityBus biz.ActivityEventBus` with `eventBus biz.EventBus` in the struct definition and constructor.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/service/... -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/service/run_status_publish.go internal/service/run_heartbeat.go internal/service/knowledge.go
git commit -m "refactor(service): migrate system-domain Publish to v2 EventBus"
```

---

### Task 10: Migrate chat-domain direct-publish (service layer, 13 sites)

**Files:**
- Modify: `internal/service/chat_event_publisher.go`
- Modify: `internal/service/chat_feedback.go`
- Modify: `internal/service/chat_confirm.go`
- Modify: `internal/service/chat_plan_confirm.go`
- Modify: `internal/service/chat_orch_await.go`
- Modify: `internal/service/chat_orchestrator_turn_phases.go`
- Modify: `internal/service/chat_orchestrator_turn_api.go`
- Modify: `internal/service/chat_run_gateway.go`
- Modify: `internal/service/graph_task_status.go`
- Modify: `internal/service/pre_planning_gate.go`
- Modify: `internal/service/spirit_team.go`

**Migration pattern** (apply to each file):

```go
// OLD (chat-domain direct-publish):
s.activityBus.Publish(ctx, biz.ActivityEvent{
    Event:    biz.ActivityEventCreated,
    Activity: biz.Activity{ID: id, Kind: biz.ActivityKindNotice, ...},
    Domain:   biz.ActivityDomainChat,
})

// NEW: publish via v2 EventBus with appropriate Event type
s.eventBus.Publish(ctx, biz.NewStepCreatedEvent(step))  // or appropriate event type
```

For chat-domain events, map v1 Activity kinds to v2 Event types:
- `ActivityKindTask` + `created/completed/failed` → `TaskCreatedEvent`/`TaskCompletedEvent`/`TaskFailedEvent`
- `ActivityKindNotice` → `SystemNoticeEvent` (or `StepCreatedEvent` with Kind=notice if it's a step)
- `ActivityKindConfirm` → `StepCreatedEvent` with Kind=confirm
- `team_stage` events → `TeamStageCreatedEvent`/`TeamStageCompletedEvent`
- `graph_stage` events → existing graph event types (if v2 equivalent exists)

- [ ] **Step 1: Migrate chat_event_publisher.go**

Replace `PublishTurnFailure` (line 82) — publishes `task.failed`:
```go
// NEW:
s.eventBus.Publish(ctx, biz.NewTaskFailedEvent(taskID, sessionID, reason))
```

- [ ] **Step 2: Migrate remaining 12 service files**

Apply the same pattern to each file. For each `activityBus.Publish` call:
1. Identify the v1 Activity kind + event type
2. Map to the corresponding v2 Event constructor
3. Replace `activityBus.Publish(ctx, biz.ActivityEvent{...})` with `eventBus.Publish(ctx, biz.NewXxxEvent(...))`
4. Replace struct field `activityBus biz.ActivityEventBus` with `eventBus biz.EventBus`

- [ ] **Step 3: Run tests**

Run: `go test ./internal/service/... -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/service/
git commit -m "refactor(service): migrate chat-domain direct-publish to v2 EventBus (13 sites)"
```

---

### Task 11: Migrate chat-domain direct-publish (team/agent/graph/biz/cronrunner, 21 sites)

**Files:**
- Modify: `internal/biz/dept_lead.go:504`
- Modify: `internal/biz/organization.go:499`
- Modify: `internal/agent/agent_factory.go:285`
- Modify: `internal/agent/task_orchestrator_impl.go` (4 sites)
- Modify: `internal/team/team_graph_run_finisher.go:167`
- Modify: `internal/team/team_graph_run_coordinator.go` (2 sites)
- Modify: `internal/team/runner_team_turn.go` (2 sites)
- Modify: `internal/team/runner_team_trpc_phases.go:180`
- Modify: `internal/team/runner_helpers.go` (3 sites)
- Modify: `internal/graph/trpc/event_bridge.go:48`
- Modify: `internal/graph/topology_evolution.go:269`
- Modify: `internal/graph/runtime_replanner.go:298`
- Modify: `internal/graph/adapter/runtime_adapter.go:253`
- Modify: `internal/cronrunner/runner.go:427`
- Modify: `internal/server/ws_message_handler.go:264`

Apply the same migration pattern as Task 10 to each file.

- [ ] **Step 1: Migrate each file**

For each `activityBus.Publish` call:
1. Map v1 Activity kind → v2 Event type
2. Replace with `eventBus.Publish(ctx, biz.NewXxxEvent(...))`
3. Update struct fields

- [ ] **Step 2: Run tests**

Run: `go test ./internal/... -count=1`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/
git commit -m "refactor: migrate remaining 21 direct-publish sites to v2 EventBus"
```

---

### Task 12: Migrate frontend system event routing

**Files:**
- Modify: `web/src/features/chat/composables/useChatWorkspace.ts`
- Delete: `web/src/features/chat/composables/useSystemEventNotification.ts`

After all backend Publish calls are migrated to v2 EventBus, system-domain events arrive as `v2_event` envelopes (via WSV2Subscriber). The frontend already has `handleV2Event` in `useChatWorkspace.ts` which routes to `useChatEventRouter → activityV2Store`.

- [ ] **Step 1: Move system event handling into v2 event router**

In `web/src/features/chat/composables/useChatEventRouter.ts` (or wherever v2 events are routed), add handling for `system.*` event kinds:

```typescript
// In the v2 event router, add cases for system events:
case 'system.run_status':
  applyRunStatus(ev);
  break;
case 'system.heartbeat':
  // Update heartbeat display
  break;
case 'system.notice':
  // Route to notice display
  break;
```

- [ ] **Step 2: Remove v1 system event routing from useChatWorkspace.ts**

In `useChatWorkspace.ts`, remove:
- `import { useSystemEventNotification }` (line 55)
- `const systemEventNotification = useSystemEventNotification(...)` (lines 153-156)
- The `systemEventNotification.handleSystemEvent(ev)` call in `handleActivityEvent` (line 180)
- The `onActivityEvent: handleActivityEvent` registration (lines 342, 510) — v2 events now arrive via `onV2Event`

- [ ] **Step 3: Delete useSystemEventNotification.ts**

Delete `web/src/features/chat/composables/useSystemEventNotification.ts` entirely.

- [ ] **Step 4: Remove v1 activity_event parsing from ws-transport.ts**

In `web/src/realtime/ws-transport.ts`, remove:
- `activity_event?: ActivityEvent` field (line 29)
- The `if (msg.activity_event) { opts.onActivityEvent?.(msg.activity_event); }` block (lines 199-202)

- [ ] **Step 5: Remove onActivityEvent from globalWsHub.ts**

In `web/src/realtime/globalWsHub.ts`, remove:
- `onActivityEvent?: (ev: ActivityEvent) => void` from the consumer interface (line 21)
- The `onActivityEvent` dispatch block (lines 65-70)

- [ ] **Step 6: Verify frontend**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
cd web && git add -A
git commit -m "refactor(web): migrate system event routing to v2 EventBus + delete useSystemEventNotification"
```

---

## Tier 4: Delete v1 code

### Task 13: Delete v1 WS subscription path

**Files:**
- Modify: `internal/server/ws_event.go`
- Modify: `internal/server/ws_io_pump.go`
- Modify: `internal/server/ws.go`

- [ ] **Step 1: Remove activity subscription from ws_event.go**

In `internal/server/ws_event.go`, remove the `activityBus.Subscribe` block (lines 58-74) and the `activityCh` return value.

- [ ] **Step 2: Remove activityEventPump from ws_io_pump.go**

In `internal/server/ws_io_pump.go`, delete the entire `activityEventPump` function (lines 150-196).

- [ ] **Step 3: Remove activity pump goroutine from ws.go**

In `internal/server/ws.go`, remove:
- `activityCh` variable (line 208)
- `safego.Go(connCtx, "ws-activity-pump", ...)` call (line 234)

- [ ] **Step 4: Remove activityBus field from WSServer**

In `internal/server/ws.go` (or wherever WSServer is defined), remove the `activityBus biz.ActivityEventBus` field and its constructor parameter.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/server/... -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/server/ws_event.go internal/server/ws_io_pump.go internal/server/ws.go
git commit -m "refactor(server): remove v1 activity_event WS subscription path"
```

---

### Task 14: Delete v1 ActivityEventBus + Wire binding

**Files:**
- Delete: `internal/event/activityevent/bus.go`
- Modify: `cmd/admin/wire.go`
- Modify: `internal/biz/activity_event.go` (delete or trim)

- [ ] **Step 1: Remove Wire binding**

In `cmd/admin/wire.go`, remove:
- `activityevent.New` provider (line 2362)
- `wire.Bind(new(biz.ActivityEventBus), new(*activityevent.Bus))` (line 2363)
- Any `activityBus` field references in provider sets

- [ ] **Step 2: Delete bus implementation**

Delete `internal/event/activityevent/bus.go` and its tests.

- [ ] **Step 3: Delete ActivityEventBus interface**

Delete `internal/biz/activity_event.go` (ActivityEvent struct + ActivityEventBus interface + ActivityEventSubscribeOptions).

Note: If any types are still referenced by tests, move them to a temporary alias or update the tests.

- [ ] **Step 4: Run wire + build**

Run: `make wire && go build ./...`
Expected: PASS

- [ ] **Step 5: Run tests**

Run: `go test ./internal/... -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: delete v1 ActivityEventBus + Wire binding"
```

---

### Task 15: Delete v1 Activity Schema + Repo + frontend types

**Files:**
- Delete: `internal/data/ent/schema/activity.go`
- Delete: `internal/data/activity_repo.go`
- Delete: `internal/biz/activity.go`
- Delete: `web/src/realtime/activityEvent.ts`
- Modify: `cmd/admin/wire.go`

- [ ] **Step 1: Remove ActivityRepo Wire binding**

In `cmd/admin/wire.go`, remove `data.NewActivityRepo` and all `wire.Bind` for `biz.ActivityReader`/`ActivityWriter`/`ActivityUpserter`/`ActivityRepo`.

- [ ] **Step 2: Delete activity_repo.go**

Delete `internal/data/activity_repo.go` and its tests.

- [ ] **Step 3: Delete Activity model + interfaces**

Delete `internal/biz/activity.go` (Activity struct + ActivityReader/Writer/Upserter/Repo interfaces).

If any code still references `biz.Activity` (e.g., the adapter in Task 7), update it to use `biz.Step` directly.

- [ ] **Step 4: Delete Ent schema**

Delete `internal/data/ent/schema/activity.go`.

Run: `go generate ./internal/data/ent`
Expected: regenerates without `activity` table

- [ ] **Step 5: Add DDL migration to drop activities table**

Create `internal/data/sql/migrations/20260704_drop_activities.sql`:

```sql
-- Drop v1 activities table (replaced by steps_v2/turns_v2/tasks_v2)
-- Idempotent: IF EXISTS prevents error on re-run
DROP TABLE IF EXISTS activities;
DROP TABLE IF EXISTS activities_fts;
```

Register it in `internal/data/ddl_migration_registry.go`.

- [ ] **Step 6: Delete frontend activityEvent.ts**

Delete `web/src/realtime/activityEvent.ts` (v1 Activity + ActivityEvent types).

Remove any remaining imports of this file across the frontend.

- [ ] **Step 7: Run wire + build + generate**

Run: `make api && make wire && go build ./...`
Expected: PASS

- [ ] **Step 8: Run tests**

Run: `go test ./internal/... -count=1`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "refactor: delete v1 Activity Schema + Repo + frontend activityEvent types"
```

---

### Task 16: Full validation

- [ ] **Step 1: Backend build + tests**

Run: `go build ./... && go test ./internal/... -count=1`
Expected: PASS

- [ ] **Step 2: Frontend build + tests**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: PASS

- [ ] **Step 3: Wire check**

Run: `make wire && go build ./cmd/admin`
Expected: PASS

- [ ] **Step 4: Verify no v1 references remain**

Run these greps — all should return zero matches:
- `grep -r "activityBus" internal/ --include="*.go"`
- `grep -r "ActivityEventBus" internal/ --include="*.go" | grep -v "_test.go"`
- `grep -r "activity_event" web/src/ --include="*.ts" --include="*.vue"`
- `grep -r "ActivityDomainSystem\|ActivityDomainChat" internal/ --include="*.go"`
- `grep -r "internal/data/ent/schema/activity.go" docs/`

Expected: no matches (or only in docs/notes)

---

## Phase 3b-E Roadmap (Future)

After Phase 3b-D completes, the remaining cleanup is:

1. **Delete v1 `session_activity_adapter.go`** — once `SessionMessageUsecase` and `SessionTimelineUsecase` read directly from v2 Step reader.
2. **Delete v1 `ListActivities` RPC** — once the frontend uses `ListStepsV2` directly (the v1 RPC currently wraps v2 for backward compat).
3. **Unify WS queue strategy** — v2 events use `enqueueSystem` (system queue), while v1 used priority-based chat queue. Verify that real-time streaming performance is not degraded.
4. **Delete `biz.Activity` type entirely** — once all adapters and conversion functions are removed.
