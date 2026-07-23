# 聊天历史懒加载实施计划（Chat History Lazy Hydration）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 打开长会话秒级可交互：全部用户指令即时渲染，执行过程仅自动水合最后一轮 + 非终态 task，历史轮次折叠为 meta-bar 卡片按需水合（滚入视口 500ms / 点击），默认停在消息底部。

**Architecture:** 后端唯一改动是 ListStepsV2 加分页参数（limit/before_seq/has_more）+ repo 分页变体 + `(session_id, seq)` 索引迁移；前端 `activityV2Store` 改为分阶段水合（Phase 1 轻量全量 + Phase 3 按需 `hydrateTask`），新增 `useLazyTaskHydration` composable（IntersectionObserver + dwell + 滚动锚定），TaskCard 增加折叠/水合中/失败/收起四态。

**Tech Stack:** Go + Kratos v2（proto）+ Ent + Postgres | Vue 3 + Pinia + Quasar + TypeScript + Vitest

**Spec:** `docs/superpowers/specs/2026-07-23-chat-history-lazy-load-design.md`（已评审）

---

## 执行环境注意（Windows）

- Go 测试如遇 C 盘空间不足：`$env:GOTMPDIR='D:\gotmp'`（或任意非系统盘）后再跑 `go test`
- 构建用 `go build ./...`（不用 `make build`）
- data 层测试需要本机 Postgres（127.0.0.1:5432）；`testhelper.SetupTestPG` 不可用时会自动 skip
- 前端命令都在 `web/` 目录下执行

---

## 文件结构

| 文件 | 职责 | 动作 |
|------|------|------|
| `api/kratos/session/v1/session.proto` | ListStepsV2 分页契约 | 修改 L685-692 |
| `api/kratos/session/v1/*.pb.go` | proto 生成物 | `make api` 重新生成 |
| `internal/biz/repo_ports_v2.go` | `StepListOptions` + `ListStepsBySessionPaged` 接口方法 | 修改 L85-98 |
| `internal/data/step_v2_repo.go` | 分页查询实现（DESC LIMIT n+1 → 反转升序） | 修改 |
| `internal/data/step_v2_repo_test.go` | PG 分页语义测试 | 新建 |
| `internal/data/sql/migrations/20261109_steps_v2_session_seq.sql` | `(session_id, seq)` 索引 | 新建 |
| `internal/data/ddl_migration_registry.go` | 注册 20261109 | 修改 L167 后 |
| `internal/service/session_v2.go` | ListSteps 参数校验 + 分页分发 | 修改 L97-120 |
| `internal/service/session_v2_test.go` | 分页参数测试 + stub 补方法 | 修改 |
| `web/src/features/session/v2Api.ts` | listStepsV2 透传 limit/before_seq | 修改 L239-247 |
| `web/src/stores/chat/activityV2Store.ts` | hydratedTaskIds/taskHydration 状态 + hydrateTask + 分阶段 fetchSessionHistory | 修改 |
| `web/src/stores/__tests__/activityV2.store.spec.ts` | 分阶段/幂等/失败/重连测试 | 修改 |
| `web/src/features/chat/composables/useLazyTaskHydration.ts` | IntersectionObserver + dwell + 锚定 + 手动折叠态 | 新建 |
| `web/src/features/chat/composables/__tests__/useLazyTaskHydration.spec.ts` | observer/dwell/锚定测试 | 新建 |
| `web/src/features/chat/composables/useActivityQueries.ts` | isTaskHydrated / taskHydrationState 门面 | 修改 |
| `web/src/features/chat/composables/useChatEventRouter.ts` | task.created 标记水合 | 修改 L24-29 |
| `web/src/components/chat/v2/TaskCard.vue` | 折叠/水合中/失败/收起四态 + meta-bar | 修改 |
| `web/src/components/chat/v2/__tests__/TaskCard.spec.ts` | 组件四态测试 | 新建 |
| `web/src/components/chat/v2/TaskList.vue` | 接入 composable，传 hydrated/collapse props | 修改 |
| `web/src/components/chat/ChatMessageList.vue` | provide 滚动容器 ref | 修改 L58-69 |
| `web/src/i18n/locales/zh-CN.ts` / `en-US.ts` | collapseExecution / loadFailedRetry | 修改 |
| `docs/development/1-chat.md` / `.design.md` / `.development.md` | DOC-SYNC 三件套 | 修改 |

---

## 后端任务

### Task 1: Proto — ListStepsV2 分页契约

**Files:**
- Modify: `api/kratos/session/v1/session.proto:685-692`

- [ ] **Step 1: 修改 proto**

将 L685-692 替换为：

```protobuf
message ListStepsV2Request {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
  string turn_id = 2;
  string task_id = 3;
  // 0 = 不分页（现状语义，全量）；>0 时仅对 session 级查询生效
  int32 limit = 4;
  // 0 = 最新窗口；>0 = 取 seq < before_seq 的上一页（向更早翻页）
  int64 before_seq = 5;
}
message ListStepsV2Response {
  // 始终按 seq 升序返回
  repeated StepV2 steps = 1;
  // limit>0 时有效：是否还有更早的 steps
  bool has_more = 2;
}
```

- [ ] **Step 2: 重新生成 + 构建**

Run: `make api && go build ./...`
Expected: exit 0；`api/kratos/session/v1/session.pb.go` 中出现 `GetLimit()` / `GetBeforeSeq()` / `GetHasMore()`

- [ ] **Step 3: Commit**

```bash
git add api/kratos/session/v1/
git commit -m "feat(api): add limit/before_seq/has_more to ListStepsV2 for chat history lazy load"
```

---

### Task 2: biz — StepListOptions + 分页接口方法

**Files:**
- Modify: `internal/biz/repo_ports_v2.go:85-98`

- [ ] **Step 1: 加 options struct + 接口方法**

在 `StepV2Reader` 接口定义前插入 options struct，并在接口内 `ListStepsBySession` 后加一行方法（L89 之后）：

```go
// StepListOptions controls paged listing of steps within a session.
// Limit<=0 means no pagination (full list, legacy semantics);
// BeforeSeq>0 returns the page with seq < BeforeSeq (walking towards older).
// Chat history lazy load (2026-07-23 design) Phase 1 uses a Limit window.
type StepListOptions struct {
	Limit     int
	BeforeSeq int64
}
```

接口内追加（紧接 `ListStepsBySession` 一行之后）：

```go
	// ListStepsBySessionPaged returns steps of a session with pagination,
	// always ordered by seq asc; hasMore reports whether older steps remain.
	// Limit<=0 degrades to the full list (hasMore=false), ordered by started_at asc.
	ListStepsBySessionPaged(ctx context.Context, sessionID string, opts StepListOptions) (steps []Step, hasMore bool, err error)
```

注：StepV2Reader 已超 5 方法（既有技术债务 DB-DEBT-05 同类），此处为懒加载必需的最小扩面，不另立窄接口。

- [ ] **Step 2: 编译验证（所有 stub 靠嵌入接口，不会断）**

Run: `go build ./...`
Expected: exit 0（`stepV2Repo` 暂未实现新方法 → 下一步补；`var _ biz.StepV2Repo` 断言会报错）

Expected: 编译失败 `stepV2Repo does not implement biz.StepV2Repo (missing ListStepsBySessionPaged method)` — 这是预期的，驱动 Task 3。

- [ ] **Step 3: Commit（与 Task 3 合并提交，见 Task 3 Step 5）**

---

### Task 3: data — 分页查询实现 + PG 测试

**Files:**
- Create: `internal/data/step_v2_repo_test.go`
- Modify: `internal/data/step_v2_repo.go`（在 `ListStepsBySession` 之后插入新方法）

- [ ] **Step 1: 写失败测试**

新建 `internal/data/step_v2_repo_test.go`：

```go
package data

import (
	"context"
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// seedPagedSteps inserts n steps (seq 1..n) into sessionID via the repo.
func seedPagedSteps(t *testing.T, repo biz.StepV2Repo, sessionID string, n int) {
	t.Helper()
	base := time.Now().UTC().Add(-time.Hour)
	for i := 1; i <= n; i++ {
		_, err := repo.CreateStep(context.Background(), biz.Step{
			ID:              fmt.Sprintf("step-%s-%d", sessionID, i),
			SessionID:       sessionID,
			SpiritSessionID: sessionID,
			Kind:            biz.StepKindReply,
			AuthorAgentKey:  "agent-1",
			Seq:             int64(i),
			Status:          biz.StepStatusCompleted,
			StartedAt:       base.Add(time.Duration(i) * time.Second),
			Version:         1,
		})
		if err != nil {
			t.Fatalf("seed step %d: %v", i, err)
		}
	}
}

func stepSeqs(steps []biz.Step) []int64 {
	out := make([]int64, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.Seq)
	}
	return out
}

func TestStepV2Repo_ListStepsBySessionPaged(t *testing.T) {
	d := newTestDataPG(t)
	repo := NewStepV2Repo(d, testNoopLogger())
	seedPagedSteps(t, repo, "sess-paged", 10)
	ctx := context.Background()

	t.Run("limit window returns latest N asc with hasMore", func(t *testing.T) {
		steps, hasMore, err := repo.ListStepsBySessionPaged(ctx, "sess-paged", biz.StepListOptions{Limit: 3})
		if err != nil {
			t.Fatalf("paged: %v", err)
		}
		if !hasMore {
			t.Error("hasMore=false, want true")
		}
		got := stepSeqs(steps)
		want := []int64{8, 9, 10}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("seqs=%v, want %v (ascending)", got, want)
		}
	})

	t.Run("before_seq walks towards older", func(t *testing.T) {
		steps, hasMore, err := repo.ListStepsBySessionPaged(ctx, "sess-paged", biz.StepListOptions{Limit: 3, BeforeSeq: 8})
		if err != nil {
			t.Fatalf("paged: %v", err)
		}
		if !hasMore {
			t.Error("hasMore=false, want true")
		}
		if got, want := fmt.Sprint(stepSeqs(steps)), "[5 6 7]"; got != want {
			t.Errorf("seqs=%s, want %s", got, want)
		}
	})

	t.Run("last page hasMore=false", func(t *testing.T) {
		steps, hasMore, err := repo.ListStepsBySessionPaged(ctx, "sess-paged", biz.StepListOptions{Limit: 3, BeforeSeq: 3})
		if err != nil {
			t.Fatalf("paged: %v", err)
		}
		if hasMore {
			t.Error("hasMore=true, want false")
		}
		if got, want := fmt.Sprint(stepSeqs(steps)), "[1 2]"; got != want {
			t.Errorf("seqs=%s, want %s", got, want)
		}
	})

	t.Run("limit=0 degrades to full list", func(t *testing.T) {
		steps, hasMore, err := repo.ListStepsBySessionPaged(ctx, "sess-paged", biz.StepListOptions{})
		if err != nil {
			t.Fatalf("paged: %v", err)
		}
		if hasMore {
			t.Error("hasMore=true, want false")
		}
		if len(steps) != 10 {
			t.Errorf("len=%d, want 10", len(steps))
		}
	})
}
```

测试需要 noop logger helper。检查 `internal/data` 是否已有 `testNoopLogger`；若无，在测试文件顶部加：

```go
func testNoopLogger() loggateway.Logger { return loggateway.NewNoop() }
```

（import `"aranea-agents/pkg/loggateway"`；若包内已有同名 helper 则复用，勿重复定义。）

- [ ] **Step 2: 跑测试确认编译失败**

Run: `go test ./internal/data/ -run TestStepV2Repo_ListStepsBySessionPaged -count=1`
Expected: 编译失败 `stepV2Repo does not implement biz.StepV2Repo`

- [ ] **Step 3: 实现 ListStepsBySessionPaged**

在 `step_v2_repo.go` 的 `ListStepsBySession` 函数（L76-88）之后插入：

```go
// ListStepsBySessionPaged returns a page of steps for the session, always
// ordered by seq asc. Query plan: WHERE session_id=? [AND seq<before_seq]
// ORDER BY seq DESC LIMIT n+1 → hasMore = (len > n) → trim → reverse to ASC.
// Limit<=0 degrades to the legacy full list (started_at asc, hasMore=false).
func (r *stepV2Repo) ListStepsBySessionPaged(ctx context.Context, sessionID string, opts biz.StepListOptions) ([]biz.Step, bool, error) {
	if r == nil || r.data == nil {
		return nil, false, fmt.Errorf("step v2 repo: database not configured")
	}
	if opts.Limit <= 0 {
		rows, err := r.data.RW().Read(ctx).StepV2.Query().
			Where(stepv2.SessionIDEQ(sessionID)).
			Order(ent.Asc(stepv2.FieldStartedAt)).
			All(ctx)
		if err != nil {
			return nil, false, entErrToBizErr(err, "STEP_V2")
		}
		return entStepsV2ToBiz(rows), false, nil
	}
	q := r.data.RW().Read(ctx).StepV2.Query().
		Where(stepv2.SessionIDEQ(sessionID))
	if opts.BeforeSeq > 0 {
		q = q.Where(stepv2.SeqLT(opts.BeforeSeq))
	}
	rows, err := q.
		Order(ent.Desc(stepv2.FieldSeq)).
		Limit(opts.Limit + 1).
		All(ctx)
	if err != nil {
		return nil, false, entErrToBizErr(err, "STEP_V2")
	}
	hasMore := len(rows) > opts.Limit
	if hasMore {
		rows = rows[:opts.Limit]
	}
	// Reverse DESC → ASC so callers always receive ascending seq order.
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return entStepsV2ToBiz(rows), hasMore, nil
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/data/ -run TestStepV2Repo_ListStepsBySessionPaged -count=1 -v`
Expected: 4 个子测试全 PASS（PG 不可用时 SKIP，可接受，但需口头确认 CI 有 PG）

- [ ] **Step 5: Commit（含 Task 2）**

```bash
git add internal/biz/repo_ports_v2.go internal/data/step_v2_repo.go internal/data/step_v2_repo_test.go
git commit -m "feat(data): add ListStepsBySessionPaged (seq DESC LIMIT n+1 → asc) for chat history lazy load"
```

---

### Task 4: data — `(session_id, seq)` 索引迁移

**Files:**
- Create: `internal/data/sql/migrations/20261109_steps_v2_session_seq.sql`
- Modify: `internal/data/ddl_migration_registry.go`（L167 `20261108` 条目之后）

- [ ] **Step 1: 新建迁移 SQL**

```sql
-- 20261109 steps_v2_session_seq: composite index for the paged session-history
-- query used by chat history lazy load Phase 1:
--   WHERE session_id = ? [AND seq < ?] ORDER BY seq DESC LIMIT n+1
-- Existing indexes cover (task_id, seq) / (spirit_session_id, seq) / (turn_id, seq)
-- but not (session_id, seq). Idempotent per DB-N6.
CREATE INDEX IF NOT EXISTS idx_steps_v2_session_seq ON steps_v2 (session_id, seq);
```

- [ ] **Step 2: 注册迁移**

在 `ddl_migration_registry.go` L167 `20261108` 条目后追加：

```go
	// 20261109 steps_v2_session_seq: composite index for chat history lazy
	// load paged session query (WHERE session_id=? ORDER BY seq DESC LIMIT n+1).
	{Version: 20261109, Name: "steps_v2_session_seq", SQL: "sql/migrations/20261109_steps_v2_session_seq.sql"},
```

- [ ] **Step 3: 构建验证**

Run: `go build ./...`
Expected: exit 0

- [ ] **Step 4: Commit**

```bash
git add internal/data/sql/migrations/20261109_steps_v2_session_seq.sql internal/data/ddl_migration_registry.go
git commit -m "feat(data): add (session_id, seq) index migration 20261109 for paged step listing"
```

---

### Task 5: service — ListSteps 参数校验 + 分页分发

**Files:**
- Modify: `internal/service/session_v2.go:97-120`
- Modify: `internal/service/session_v2_test.go`

- [ ] **Step 1: 写失败测试（stub 加分页方法 + 3 个新用例）**

在 `session_v2_test.go` 的 `stubStepV2Reader`（L68-98）上追加字段与方法：

```go
type stubStepV2Reader struct {
	biz.StepV2Reader
	steps       []biz.Step
	pagedOpts   *biz.StepListOptions // records last paged call opts
	pagedHasMore bool
}

func (s *stubStepV2Reader) ListStepsBySessionPaged(_ context.Context, _ string, opts biz.StepListOptions) ([]biz.Step, bool, error) {
	o := opts
	s.pagedOpts = &o
	return s.steps, s.pagedHasMore, nil
}
```

（注意：原 struct 定义只有 `steps` 字段，替换整个 struct 定义为上式。）

文件末尾追加测试：

```go
// TestSessionV2Service_ListSteps_Paged verifies limit>0 routes to the paged
// repo method, clamps to the server cap, and propagates has_more.
func TestSessionV2Service_ListSteps_Paged(t *testing.T) {
	stub := &stubStepV2Reader{
		steps: []biz.Step{
			{ID: "s1", SessionID: "sess1", Kind: biz.StepKindReply, Content: "hello"},
		},
		pagedHasMore: true,
	}
	svc := &SessionV2Service{stepReader: stub}
	resp, err := svc.ListSteps(context.Background(), &v1.ListStepsV2Request{
		SessionId: "sess1",
		Limit:     100,
		BeforeSeq: 42,
	})
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if stub.pagedOpts == nil {
		t.Fatal("paged repo method not called")
	}
	if stub.pagedOpts.Limit != 100 || stub.pagedOpts.BeforeSeq != 42 {
		t.Errorf("opts=%+v, want {Limit:100 BeforeSeq:42}", stub.pagedOpts)
	}
	if !resp.GetHasMore() {
		t.Error("has_more=false, want true")
	}
}

// TestSessionV2Service_ListSteps_PagedLimitClamp verifies limit>500 is clamped.
func TestSessionV2Service_ListSteps_PagedLimitClamp(t *testing.T) {
	stub := &stubStepV2Reader{}
	svc := &SessionV2Service{stepReader: stub}
	if _, err := svc.ListSteps(context.Background(), &v1.ListStepsV2Request{SessionId: "sess1", Limit: 99999}); err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if stub.pagedOpts == nil || stub.pagedOpts.Limit != 500 {
		t.Errorf("opts=%+v, want Limit clamped to 500", stub.pagedOpts)
	}
}

// TestSessionV2Service_ListSteps_LegacyWhenNoLimit verifies limit=0 keeps the
// legacy full-list path and never touches the paged method.
func TestSessionV2Service_ListSteps_LegacyWhenNoLimit(t *testing.T) {
	stub := &stubStepV2Reader{steps: []biz.Step{{ID: "s1", SessionID: "sess1"}}}
	svc := &SessionV2Service{stepReader: stub}
	resp, err := svc.ListSteps(context.Background(), &v1.ListStepsV2Request{SessionId: "sess1"})
	if err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if stub.pagedOpts != nil {
		t.Error("paged method must not be called when limit=0")
	}
	if resp.GetHasMore() {
		t.Error("has_more=true, want false on legacy path")
	}
}

// TestSessionV2Service_ListSteps_RejectsNegativeParams verifies 400 on negative
// limit / before_seq.
func TestSessionV2Service_ListSteps_RejectsNegativeParams(t *testing.T) {
	svc := &SessionV2Service{stepReader: &stubStepV2Reader{}}
	if _, err := svc.ListSteps(context.Background(), &v1.ListStepsV2Request{SessionId: "s", Limit: -1}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Errorf("limit=-1: want BadRequest, got %v", err)
	}
	if _, err := svc.ListSteps(context.Background(), &v1.ListStepsV2Request{SessionId: "s", BeforeSeq: -1}); !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Errorf("before_seq=-1: want BadRequest, got %v", err)
	}
}

// TestSessionV2Service_ListSteps_TaskBranchIgnoresPaging verifies task_id branch
// keeps the legacy per-task full list (design: 单 task 有界，不分页).
func TestSessionV2Service_ListSteps_TaskBranchIgnoresPaging(t *testing.T) {
	stub := &stubStepV2Reader{steps: []biz.Step{{ID: "s1", TaskID: "t1"}}}
	svc := &SessionV2Service{stepReader: stub}
	if _, err := svc.ListSteps(context.Background(), &v1.ListStepsV2Request{SessionId: "sess1", TaskId: "t1", Limit: 5}); err != nil {
		t.Fatalf("ListSteps: %v", err)
	}
	if stub.pagedOpts != nil {
		t.Error("paged method must not be called for task_id branch")
	}
}
```

注：`stubStepV2Reader` 现有 `ListStepsByTask` 未 stub——task 分支会调到嵌入接口的 nil 方法 panic。需在 stub 上补：

```go
func (s *stubStepV2Reader) ListStepsByTask(_ context.Context, _ string) ([]biz.Step, error) {
	return s.steps, nil
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `go test ./internal/service/ -run 'TestSessionV2Service_ListSteps' -count=1`
Expected: 编译失败（`stubStepV2Reader.pagedOpts` undefined 字段冲突无——应编译过但新用例 FAIL：`paged repo method not called` / `has_more` 不存在若 Task 1 未做）。确认 Task 1 已完成后，失败原因是 service 未分发分页路径。

- [ ] **Step 3: 实现 service 分发**

将 `session_v2.go` L97-120 的 `ListSteps` 替换为：

```go
// listStepsMaxLimit caps client-supplied page sizes to protect the DB.
const listStepsMaxLimit = 500

func (s *SessionV2Service) ListSteps(ctx context.Context, req *v1.ListStepsV2Request) (*v1.ListStepsV2Response, error) {
	if req.GetLimit() < 0 || req.GetBeforeSeq() < 0 {
		return nil, apierror.BadRequest(apierror.DomainShared, "limit and before_seq must be >= 0")
	}
	var steps []biz.Step
	var err error
	var hasMore bool
	switch {
	case strings.TrimSpace(req.GetTurnId()) != "":
		steps, err = s.stepReader.ListStepsByTurn(ctx, req.GetTurnId())
	case strings.TrimSpace(req.GetTaskId()) != "":
		steps, err = s.stepReader.ListStepsByTask(ctx, req.GetTaskId())
	default:
		sessionID := strings.TrimSpace(req.GetSessionId())
		if sessionID == "" {
			return nil, apierror.BadRequest(apierror.DomainShared, "session_id is required when turn_id and task_id are empty")
		}
		if req.GetLimit() > 0 {
			limit := int(req.GetLimit())
			if limit > listStepsMaxLimit {
				limit = listStepsMaxLimit
			}
			steps, hasMore, err = s.stepReader.ListStepsBySessionPaged(ctx, sessionID, biz.StepListOptions{
				Limit:     limit,
				BeforeSeq: req.GetBeforeSeq(),
			})
		} else {
			steps, err = s.stepReader.ListStepsBySession(ctx, sessionID)
		}
	}
	if err != nil {
		return nil, err
	}
	out := make([]*v1.StepV2, 0, len(steps))
	for _, st := range steps {
		out = append(out, bizStepToProto(st))
	}
	return &v1.ListStepsV2Response{Steps: out, HasMore: hasMore}, nil
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/service/ -run 'TestSessionV2Service_ListSteps' -count=1 -v`
Expected: 全部 PASS（含旧用例）

- [ ] **Step 5: Commit**

```bash
git add internal/service/session_v2.go internal/service/session_v2_test.go
git commit -m "feat(service): ListSteps paged dispatch (limit<=500, has_more) with legacy limit=0 path"
```

---

## 前端任务

### Task 6: v2Api — listStepsV2 透传分页参数

**Files:**
- Modify: `web/src/features/session/v2Api.ts:239-247`

- [ ] **Step 1: 修改函数签名与参数透传**

将 L236-247 替换为：

```typescript
/**
 * List steps for a session. Backend: GET /v2/sessions/{session_id}/steps.
 * Optional `turnId` / `taskId` filters are passed as query params.
 * `limit` / `beforeSeq` enable the paged session-window query (chat history
 * lazy load Phase 1); has_more is currently unused by the frontend (YAGNI —
 * spirit-level orphan steps beyond the window are not expected to exist).
 */
export async function listStepsV2(
  sessionId: string,
  opts?: { turnId?: string; taskId?: string; limit?: number; beforeSeq?: number },
): Promise<Step[]> {
  const params: Record<string, string | number> = {};
  if (opts?.turnId) params.turn_id = opts.turnId;
  if (opts?.taskId) params.task_id = opts.taskId;
  if (opts?.limit && opts.limit > 0) params.limit = opts.limit;
  if (opts?.beforeSeq && opts.beforeSeq > 0) params.before_seq = opts.beforeSeq;
  const resp = await kratosApi.get<ListStepsV2Response>(`/v2/sessions/${encodeURIComponent(sessionId)}/steps`, {
    params,
  });
  return (resp.data?.steps ?? []).map(mapStep);
}
```

- [ ] **Step 2: 类型检查**

Run: `cd web && pnpm lint`
Expected: exit 0（现有调用方签名兼容——第二参数为可选 opts 对象，仅新增可选字段）

- [ ] **Step 3: Commit（与 Task 7/8 合并提交，见 Task 8 Step 5）**

---

### Task 7: store — 水合状态 + upsert 不变式 + WS task.created 标记

**Files:**
- Modify: `web/src/stores/chat/activityV2Store.ts`
- Modify: `web/src/features/chat/composables/useChatEventRouter.ts:24-29`
- Modify: `web/src/stores/__tests__/activityV2.store.spec.ts`
- Modify: `web/src/features/chat/composables/__tests__/useChatEventRouter.spec.ts`

- [ ] **Step 1: 写失败测试**

在 `activityV2.store.spec.ts` 末尾追加：

```typescript
it('hydratedTaskIds starts empty and survives upsertTask (no auto-mark on bulk path)', () => {
  const s = useChatActivityStore();
  s.upsertTask(makeTask({ ID: 't1' }));
  // upsertTask 不自动标记——历史任务经 fetchSessionHistory 批量 upsert，不能误判为水合。
  expect(s.hydratedTaskIds.has('t1')).toBe(false);
});

it('clearAll resets hydration tracking', () => {
  const s = useChatActivityStore();
  s.hydratedTaskIds.add('t1');
  s.taskHydration.set('t1', 'loading');
  s.clearAll();
  expect(s.hydratedTaskIds.size).toBe(0);
  expect(s.taskHydration.size).toBe(0);
});
```

在 `useChatEventRouter.spec.ts` 追加（沿用该文件既有测试结构）：

```typescript
it('task.created marks the new task as hydrated (live tasks default expanded)', () => {
  const s = useChatActivityStore();
  const router = useChatEventRouter(s);
  router.dispatch({
    type: 'v2_event',
    kind: 'task.created',
    payload: { Task: makeTask({ ID: 't-live' }) },
  } as never);
  expect(s.hydratedTaskIds.has('t-live')).toBe(true);
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd web && pnpm vitest run src/stores/__tests__/activityV2.store.spec.ts src/features/chat/composables/__tests__/useChatEventRouter.spec.ts`
Expected: FAIL — `s.hydratedTaskIds is undefined` / `taskHydration is undefined`

- [ ] **Step 3: 实现 store 状态 + router 标记**

`activityV2Store.ts`：在 `loadedMemberStepSessions` 声明（L61）之后追加：

```typescript
  // === Lazy hydration state (chat history lazy load, 2026-07-23 design) ===
  // hydratedTaskIds: tasks whose execution subtree has been loaded. Persists
  // across WS reconnects (fetchSessionHistory never clears it) so expanded
  // cards stay expanded; only clearAll/clearSession reset it.
  const hydratedTaskIds = ref(new Set<string>());
  // taskHydration: transient per-task fetch state ('loading' | 'error').
  const taskHydration = ref(new Map<string, 'loading' | 'error'>());
```

`clearAll()` 内（L516 `loadedMemberStepSessions.value.clear()` 之后）追加：

```typescript
    hydratedTaskIds.value.clear();
    taskHydration.value.clear();
```

`clearSession(spiritSessionId)` 内，在删除 tasks 的循环里同步清理水合状态——将该函数第一个循环（L468-470）改为：

```typescript
    for (const [id, t] of tasks.value) {
      if (t.SessionID === spiritSessionId) {
        tasks.value.delete(id);
        hydratedTaskIds.value.delete(id);
        taskHydration.value.delete(id);
      }
    }
```

store return 对象（L623-666）的 `hydrationErrors,` 之后追加导出：

```typescript
    hydratedTaskIds,
    taskHydration,
```

`useChatEventRouter.ts`：将 `task.created` 分支（L24-28）从合并 case 中拆出：

```typescript
      // Task events
      case 'task.created':
        if (p.Task) {
          store.upsertTask(p.Task as never);
          // 会话进行中新建的 task 默认展开（活跃任务永远自动水合，设计 P5/§5）。
          store.hydratedTaskIds.add((p.Task as { ID: string }).ID);
        }
        break;
      case 'task.updated':
      case 'task.completed':
      case 'task.failed':
        if (p.Task) store.upsertTask(p.Task as never);
        break;
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd web && pnpm vitest run src/stores/__tests__/activityV2.store.spec.ts src/features/chat/composables/__tests__/useChatEventRouter.spec.ts`
Expected: PASS

- [ ] **Step 5: Commit（与 Task 8 合并，见 Task 8 Step 5）**

---

### Task 8: store — hydrateTask + 分阶段 fetchSessionHistory

**Files:**
- Modify: `web/src/stores/chat/activityV2Store.ts`（L519-600 重写 + 新增 hydrateTask）
- Modify: `web/src/stores/__tests__/activityV2.store.spec.ts`

**前置**: Task 6/7 已合入工作区（本任务依赖其状态与 API 签名）。

- [ ] **Step 1: 写失败测试**

在 `activityV2.store.spec.ts` 追加（需要 `import { flushPromises } from '@vue/test-utils';` 于文件顶部）：

```typescript
describe('fetchSessionHistory phased hydration', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(listOrphanMemberSessionsV2).mockResolvedValue([]);
  });

  it('Phase 1 fetches steps with limit window, not full list', async () => {
    vi.mocked(listTasksV2).mockResolvedValue([]);
    vi.mocked(listStepsV2).mockResolvedValue([]);
    const s = useChatActivityStore();
    await s.fetchSessionHistory('sess-1');
    expect(listStepsV2).toHaveBeenCalledWith('sess-1', { limit: 100 });
  });

  it('auto-hydrates only last + non-terminal tasks; terminal history stays collapsed', async () => {
    vi.mocked(listTasksV2).mockResolvedValue([
      makeTask({ ID: 't-old', SessionID: 'sess-1', Status: 'completed', Seq: 1 }),
      makeTask({ ID: 't-mid', SessionID: 'sess-1', Status: 'completed', Seq: 2 }),
      makeTask({ ID: 't-last', SessionID: 'sess-1', Status: 'completed', Seq: 3 }),
    ]);
    vi.mocked(listStepsV2).mockResolvedValue([]);
    vi.mocked(listTurnsV2).mockResolvedValue([]);
    vi.mocked(listTeamStagesV2).mockResolvedValue([]);
    vi.mocked(listPlanBoardsV2).mockResolvedValue([]);
    vi.mocked(listPlanStepsV2).mockResolvedValue([]);
    vi.mocked(listGraphStagesV2).mockResolvedValue([]);

    const s = useChatActivityStore();
    await s.fetchSessionHistory('sess-1');
    await flushPromises();

    // 只有最后一个 task 被自动水合 → listTurnsV2 只调用 1 次
    expect(listTurnsV2).toHaveBeenCalledTimes(1);
    expect(listTurnsV2).toHaveBeenCalledWith('t-last');
    expect(s.hydratedTaskIds.has('t-last')).toBe(true);
    expect(s.hydratedTaskIds.has('t-old')).toBe(false);
    expect(s.hydratedTaskIds.has('t-mid')).toBe(false);
  });

  it('auto-hydrates non-terminal tasks (running/pending/interrupted) regardless of position', async () => {
    vi.mocked(listTasksV2).mockResolvedValue([
      makeTask({ ID: 't-run', SessionID: 'sess-1', Status: 'running', Seq: 1 }),
      makeTask({ ID: 't-int', SessionID: 'sess-1', Status: 'interrupted', Seq: 2 }),
      makeTask({ ID: 't-done', SessionID: 'sess-1', Status: 'completed', Seq: 3 }),
    ]);
    vi.mocked(listStepsV2).mockResolvedValue([]);
    vi.mocked(listTurnsV2).mockResolvedValue([]);
    vi.mocked(listTeamStagesV2).mockResolvedValue([]);
    vi.mocked(listPlanBoardsV2).mockResolvedValue([]);
    vi.mocked(listPlanStepsV2).mockResolvedValue([]);
    vi.mocked(listGraphStagesV2).mockResolvedValue([]);

    const s = useChatActivityStore();
    await s.fetchSessionHistory('sess-1');
    await flushPromises();

    const hydrated = new Set(vi.mocked(listTurnsV2).mock.calls.map((c) => c[0]));
    expect(hydrated).toEqual(new Set(['t-run', 't-int', 't-done'])); // t-done 是最后一个
  });

  it('hydratedTaskIds survive a re-fetch (WS reconnect keeps cards expanded)', async () => {
    vi.mocked(listTasksV2).mockResolvedValue([makeTask({ ID: 't-1', SessionID: 'sess-1', Status: 'completed' })]);
    vi.mocked(listStepsV2).mockResolvedValue([]);
    vi.mocked(listTurnsV2).mockResolvedValue([]);
    vi.mocked(listTeamStagesV2).mockResolvedValue([]);
    vi.mocked(listPlanBoardsV2).mockResolvedValue([]);
    vi.mocked(listPlanStepsV2).mockResolvedValue([]);
    vi.mocked(listGraphStagesV2).mockResolvedValue([]);

    const s = useChatActivityStore();
    await s.fetchSessionHistory('sess-1');
    await flushPromises();
    expect(s.hydratedTaskIds.has('t-1')).toBe(true);

    // 重连再拉：即便 t-1 已非「最后+非终态」之外的逻辑，仍因已水合而保持
    await s.fetchSessionHistory('sess-1');
    await flushPromises();
    expect(s.hydratedTaskIds.has('t-1')).toBe(true);
  });
});

describe('hydrateTask', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(listOrphanMemberSessionsV2).mockResolvedValue([]);
  });

  function seedOneCompletedTask(): ReturnType<typeof useChatActivityStore> {
    vi.mocked(listTasksV2).mockResolvedValue([makeTask({ ID: 't-1', SessionID: 'sess-1', Status: 'completed' })]);
    vi.mocked(listStepsV2).mockResolvedValue([]);
    const s = useChatActivityStore();
    return s;
  }

  it('is idempotent: concurrent calls trigger only one fetch round', async () => {
    const s = seedOneCompletedTask();
    vi.mocked(listTurnsV2).mockResolvedValue([]);
    vi.mocked(listTeamStagesV2).mockResolvedValue([]);
    vi.mocked(listPlanBoardsV2).mockResolvedValue([]);
    vi.mocked(listPlanStepsV2).mockResolvedValue([]);
    vi.mocked(listGraphStagesV2).mockResolvedValue([]);

    // 注意：t-1 是最后一个 task，fetchSessionHistory 已触发一次 hydrate；
    // 这里直接测 store action 本身——先造一个未水合的 task。
    const s2 = useChatActivityStore();
    s2.upsertTask(makeTask({ ID: 't-9', SessionID: 'sess-1', Status: 'completed' }));
    await Promise.all([s2.hydrateTask('t-9'), s2.hydrateTask('t-9')]);
    const calls = vi.mocked(listTurnsV2).mock.calls.filter((c) => c[0] === 't-9');
    expect(calls.length).toBe(1);
    expect(s2.hydratedTaskIds.has('t-9')).toBe(true);
    void s;
  });

  it('sets error state on sub-resource failure and allows retry', async () => {
    const s = useChatActivityStore();
    s.upsertTask(makeTask({ ID: 't-9', SessionID: 'sess-1', Status: 'completed' }));

    vi.mocked(listTurnsV2).mockRejectedValueOnce(new Error('turns down'));
    vi.mocked(listStepsV2).mockResolvedValue([]);
    vi.mocked(listTeamStagesV2).mockResolvedValue([]);
    vi.mocked(listPlanBoardsV2).mockResolvedValue([]);
    vi.mocked(listPlanStepsV2).mockResolvedValue([]);
    vi.mocked(listGraphStagesV2).mockResolvedValue([]);

    await s.hydrateTask('t-9');
    expect(s.taskHydration.get('t-9')).toBe('error');
    expect(s.hydratedTaskIds.has('t-9')).toBe(false);

    // 重试成功
    vi.mocked(listTurnsV2).mockResolvedValue([]);
    await s.hydrateTask('t-9');
    expect(s.taskHydration.get('t-9')).toBeUndefined();
    expect(s.hydratedTaskIds.has('t-9')).toBe(true);
  });
});
```

同时修复既有测试 `fetchSessionHistory records sub-resource failures in hydrationErrors`（L128-145）——per-task 抓取变为 fire-and-forget，断言前需 flush：

```typescript
    const s = useChatActivityStore();
    await s.fetchSessionHistory('sess-1');
    await flushPromises(); // per-task hydration is fire-and-forget now

    expect(s.hydrationErrors.length).toBe(1);
```

（该用例 task 状态为 running → 会被自动水合，turns 失败被记录。）

- [ ] **Step 2: 跑测试确认失败**

Run: `cd web && pnpm vitest run src/stores/__tests__/activityV2.store.spec.ts`
Expected: FAIL — `s.hydrateTask is not a function` / Phase 1 limit 断言失败

- [ ] **Step 3: 实现分阶段 fetchSessionHistory + hydrateTask**

`activityV2Store.ts`：在 `catchHydrationError` 之后（L72 附近）加常量：

```typescript
// Phase 1 recent-steps window — covers spirit-level orphan steps and gives
// the latest task immediate context (design §4.2 Phase 1).
const HISTORY_STEP_WINDOW = 100;
// Non-terminal task statuses always auto-hydrate on session open (P5):
// running/pending 进行态 + interrupted（「继续执行」按钮必须直接可见）。
const AUTO_HYDRATE_STATUSES = new Set<Task['Status']>(['pending', 'running', 'interrupted']);
```

将现有 `fetchSessionHistory`（L530-600）整体替换为：

```typescript
  /**
   * fetchSessionHistory loads the v2 entity tree in phases (chat history lazy
   * load, 2026-07-23 design §4.2):
   *   Phase 1: tasks (lightweight, all) + recent steps window (limit=100)
   *   Phase 2: compute auto-hydrate set = last task + non-terminal tasks
   *            + already-hydrated tasks (WS reconnect keeps them expanded)
   *   Phase 3: hydrateTask per auto-hydrate task (parallel, fire-and-forget)
   *   Mode B: orphan member_sessions by spirit session (session-level)
   * Historical terminal tasks render as collapsed meta-bar cards and hydrate
   * on demand via hydrateTask (viewport dwell / click).
   */
  async function fetchSessionHistory(sessionId: string): Promise<void> {
    // P2-07: clear previous hydration errors at the start of each fetch.
    hydrationErrors.value = [];

    const [tasksList, stepsList] = await Promise.all([
      listTasksV2(sessionId),
      listStepsV2(sessionId, { limit: HISTORY_STEP_WINDOW }),
    ]);
    for (const t of tasksList) upsertTask(t);
    for (const s of stepsList) upsertStep(s);

    const sorted = [...tasksList].sort(compareByTimeThenSeq);
    const autoHydrate = new Set<string>();
    const lastTask = sorted[sorted.length - 1];
    if (lastTask) autoHydrate.add(lastTask.ID);
    for (const t of sorted) {
      if (AUTO_HYDRATE_STATUSES.has(t.Status)) autoHydrate.add(t.ID);
      if (hydratedTaskIds.value.has(t.ID)) autoHydrate.add(t.ID);
    }
    // Fire-and-forget: 首屏不等待执行过程水合，折叠卡即时渲染。
    for (const id of autoHydrate) {
      void hydrateTask(id);
    }

    // Mode B: orphan member sessions (empty TeamRunID) for this spirit session.
    const orphans = await listOrphanMemberSessionsV2(sessionId).catch(
      catchHydrationError<MemberSession>('orphan_member_sessions', sessionId),
    );
    for (const ms of orphans) upsertMemberSession(ms);
  }

  /**
   * hydrateTask loads one task's full execution subtree (turns + task steps +
   * team/plan/graph entities + drill-down runs/sessions/nodes). Idempotent:
   * returns immediately when already hydrated or in flight. On any
   * sub-resource failure the task enters 'error' state (meta-bar retry) and
   * is NOT marked hydrated, so a later expand retries the full fetch.
   */
  async function hydrateTask(taskId: string): Promise<void> {
    const task = tasks.value.get(taskId);
    if (!task) return;
    if (hydratedTaskIds.value.has(taskId)) return;
    if (taskHydration.value.get(taskId) === 'loading') return;
    taskHydration.value.set(taskId, 'loading');

    const errorsBefore = hydrationErrors.value.length;
    const [turnsL, stepsL, teamStagesL, planBoardsL, planStepsL, graphStagesL] = await Promise.all([
      listTurnsV2(taskId).catch(catchHydrationError<Turn>('turns', taskId)),
      listStepsV2(task.SessionID, { taskId }).catch(catchHydrationError<Step>('steps', taskId)),
      listTeamStagesV2(taskId).catch(catchHydrationError<TeamStage>('team_stages', taskId)),
      listPlanBoardsV2(taskId).catch(catchHydrationError<PlanBoard>('plan_boards', taskId)),
      listPlanStepsV2(taskId).catch(catchHydrationError<PlanStep>('plan_steps', taskId)),
      listGraphStagesV2(taskId).catch(catchHydrationError<GraphStage>('graph_stages', taskId)),
    ]);
    for (const turn of turnsL) upsertTurn(turn);
    for (const st of stepsL) upsertStep(st);
    for (const ts of teamStagesL) upsertTeamStage(ts);
    for (const pb of planBoardsL) upsertPlanBoard(pb);
    for (const ps of planStepsL) upsertPlanStep(ps);
    for (const gs of graphStagesL) upsertGraphStage(gs);

    // Drill-down: team_runs → member_sessions (metadata only; member step
    // content stays lazy per A.4.7), graph_nodes per graph_stage.
    const teamRunLists = await Promise.all(
      teamStagesL.map((ts) => listTeamRunsV2(ts.ID).catch(catchHydrationError<TeamRun>('team_runs', ts.ID))),
    );
    const allTeamRuns: TeamRun[] = [];
    for (const runs of teamRunLists) {
      for (const tr of runs) upsertTeamRun(tr);
      allTeamRuns.push(...runs);
    }
    const memberSessionLists = await Promise.all(
      allTeamRuns.map((tr) =>
        listMemberSessionsV2(tr.ID).catch(catchHydrationError<MemberSession>('member_sessions', tr.ID)),
      ),
    );
    for (const sessions of memberSessionLists) {
      for (const ms of sessions) upsertMemberSession(ms);
    }
    const graphNodeLists = await Promise.all(
      graphStagesL.map((gs) => listGraphNodesV2(gs.ID).catch(catchHydrationError<GraphNode>('graph_nodes', gs.ID))),
    );
    for (const nodes of graphNodeLists) {
      for (const gn of nodes) upsertGraphNode(gn);
    }

    if (hydrationErrors.value.length > errorsBefore) {
      taskHydration.value.set(taskId, 'error');
    } else {
      hydratedTaskIds.value.add(taskId);
      taskHydration.value.delete(taskId);
    }
  }
```

store return 对象中，`fetchSessionHistory,` 之后加 `hydrateTask,`。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd web && pnpm vitest run src/stores/__tests__/activityV2.store.spec.ts`
Expected: 全部 PASS

- [ ] **Step 5: Commit（含 Task 6/7）**

```bash
git add web/src/features/session/v2Api.ts web/src/stores/chat/activityV2Store.ts web/src/features/chat/composables/useChatEventRouter.ts web/src/stores/__tests__/activityV2.store.spec.ts web/src/features/chat/composables/__tests__/useChatEventRouter.spec.ts
git commit -m "feat(web): phased session history hydration + hydrateTask lazy action in activityV2Store"
```

---

### Task 9: composable — useLazyTaskHydration

**Files:**
- Create: `web/src/features/chat/composables/useLazyTaskHydration.ts`
- Create: `web/src/features/chat/composables/__tests__/useLazyTaskHydration.spec.ts`

- [ ] **Step 1: 写失败测试**

新建 `useLazyTaskHydration.spec.ts`：

```typescript
// web/src/features/chat/composables/__tests__/useLazyTaskHydration.spec.ts
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { ref } from 'vue';
import { useLazyTaskHydration, HYDRATE_DWELL_MS } from '../useLazyTaskHydration';

type IOCallback = (entries: Array<Partial<IntersectionObserverEntry>>) => void;

let ioCallback: IOCallback | null = null;
const observedEls = new Set<Element>();

class MockIntersectionObserver {
  constructor(cb: IOCallback, _opts?: IntersectionObserverInit) {
    ioCallback = cb;
  }
  observe(el: Element) {
    observedEls.add(el);
  }
  unobserve(el: Element) {
    observedEls.delete(el);
  }
  disconnect() {
    observedEls.clear();
  }
}

function makeScrollEl(): HTMLElement {
  const root = document.createElement('div');
  for (const id of ['t-1', 't-2', 't-3']) {
    const card = document.createElement('div');
    card.className = 'task-card';
    card.dataset.taskId = id;
    root.appendChild(card);
  }
  return root;
}

function entry(taskId: string, isIntersecting: boolean): Partial<IntersectionObserverEntry> {
  const target = document.querySelector(`.task-card[data-task-id="${taskId}"]`) as Element;
  return { target, isIntersecting };
}

describe('useLazyTaskHydration', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    observedEls.clear();
    ioCallback = null;
    vi.stubGlobal('IntersectionObserver', MockIntersectionObserver);
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    document.body.innerHTML = '';
  });

  it('syncCards observes only cards that need hydration', () => {
    const scrollEl = ref(makeScrollEl());
    document.body.appendChild(scrollEl.value!);
    const needsHydration = (id: string) => id !== 't-2'; // t-2 已水合
    const lazy = useLazyTaskHydration({ scrollEl, needsHydration, hydrate: vi.fn() });
    lazy.syncCards();
    expect([...observedEls].map((el) => (el as HTMLElement).dataset.taskId).sort()).toEqual(['t-1', 't-3']);
  });

  it('fires hydrate after 500ms dwell inside viewport', () => {
    const scrollEl = ref(makeScrollEl());
    document.body.appendChild(scrollEl.value!);
    const hydrate = vi.fn().mockResolvedValue(undefined);
    const lazy = useLazyTaskHydration({ scrollEl, needsHydration: () => true, hydrate });
    lazy.syncCards();

    ioCallback!([entry('t-1', true)]);
    expect(hydrate).not.toHaveBeenCalled();
    vi.advanceTimersByTime(HYDRATE_DWELL_MS - 1);
    expect(hydrate).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(hydrate).toHaveBeenCalledWith('t-1');
  });

  it('cancels dwell when the card leaves the viewport (fast scroll-by)', () => {
    const scrollEl = ref(makeScrollEl());
    document.body.appendChild(scrollEl.value!);
    const hydrate = vi.fn().mockResolvedValue(undefined);
    const lazy = useLazyTaskHydration({ scrollEl, needsHydration: () => true, hydrate });
    lazy.syncCards();

    ioCallback!([entry('t-1', true)]);
    vi.advanceTimersByTime(200);
    ioCallback!([entry('t-1', false)]);
    vi.advanceTimersByTime(HYDRATE_DWELL_MS);
    expect(hydrate).not.toHaveBeenCalled();
  });

  it('expandTask hydrates immediately without dwell', () => {
    const scrollEl = ref(makeScrollEl());
    document.body.appendChild(scrollEl.value!);
    const hydrate = vi.fn().mockResolvedValue(undefined);
    const lazy = useLazyTaskHydration({ scrollEl, needsHydration: () => true, hydrate });
    lazy.expandTask('t-3');
    expect(hydrate).toHaveBeenCalledWith('t-3');
  });

  it('compensates scrollTop when a card above the viewport expands', async () => {
    const scrollEl = ref(makeScrollEl());
    document.body.appendChild(scrollEl.value!);
    const el = scrollEl.value!;
    // 视口 top=0；t-1 卡片 top=-300（在视口上方）
    vi.spyOn(el, 'getBoundingClientRect').mockReturnValue({ top: 0 } as DOMRect);
    const card = el.querySelector<HTMLElement>('.task-card[data-task-id="t-1"]')!;
    vi.spyOn(card, 'getBoundingClientRect').mockReturnValue({ top: -300 } as DOMRect);
    let scrollHeight = 1000;
    Object.defineProperty(el, 'scrollHeight', { get: () => scrollHeight, configurable: true });
    el.scrollTop = 400;

    const hydrate = vi.fn().mockImplementation(async () => {
      scrollHeight = 1800; // 水合后 DOM 增高 800
    });
    const lazy = useLazyTaskHydration({ scrollEl, needsHydration: () => true, hydrate });
    lazy.expandTask('t-1');
    await vi.waitFor(() => expect(el.scrollTop).toBe(1200)); // 400 + 800
  });

  it('does not compensate scrollTop when the card is inside the viewport', async () => {
    const scrollEl = ref(makeScrollEl());
    document.body.appendChild(scrollEl.value!);
    const el = scrollEl.value!;
    vi.spyOn(el, 'getBoundingClientRect').mockReturnValue({ top: 0 } as DOMRect);
    const card = el.querySelector<HTMLElement>('.task-card[data-task-id="t-1"]')!;
    vi.spyOn(card, 'getBoundingClientRect').mockReturnValue({ top: 200 } as DOMRect);
    Object.defineProperty(el, 'scrollHeight', { get: () => 1000, configurable: true });
    el.scrollTop = 400;

    const hydrate = vi.fn().mockResolvedValue(undefined);
    const lazy = useLazyTaskHydration({ scrollEl, needsHydration: () => true, hydrate });
    lazy.expandTask('t-1');
    await Promise.resolve();
    expect(el.scrollTop).toBe(400);
  });

  it('toggleCollapse tracks manual collapse state', () => {
    const scrollEl = ref(makeScrollEl());
    const lazy = useLazyTaskHydration({ scrollEl, needsHydration: () => true, hydrate: vi.fn() });
    expect(lazy.isCollapsed('t-1')).toBe(false);
    lazy.toggleCollapse('t-1');
    expect(lazy.isCollapsed('t-1')).toBe(true);
    lazy.toggleCollapse('t-1');
    expect(lazy.isCollapsed('t-1')).toBe(false);
  });
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd web && pnpm vitest run src/features/chat/composables/__tests__/useLazyTaskHydration.spec.ts`
Expected: FAIL — 模块不存在

- [ ] **Step 3: 实现 composable**

新建 `web/src/features/chat/composables/useLazyTaskHydration.ts`：

```typescript
/**
 * useLazyTaskHydration — 历史 task 折叠卡的懒水合编排（2026-07-23 设计 §4.3）。
 *
 * - IntersectionObserver（root=消息滚动容器, threshold=0.4）：折叠卡进入视口
 *   启动 500ms dwell 定时器 → hydrate；离开视口取消（快速滑过不触发）。
 * - expandTask(taskId)：点击卡片立即水合。
 * - 滚动锚定：卡片原位置在视口上方时，水合渲染后按 scrollHeight 增量补偿
 *   scrollTop，视口不跳动。
 * - collapsedIds：水合后手动「收起执行过程」的 UI 态；数据保留 store，
 *   再展开零请求（设计 P6）。
 *
 * 网络请求不在这里——hydrate 由调用方注入（store action），本 composable
 * 只做 DOM 编排，便于单测 mock。
 */
import { nextTick, onBeforeUnmount, ref, type Ref } from 'vue';

export const HYDRATE_DWELL_MS = 500;
const IO_THRESHOLD = 0.4;

/** ChatMessageList provide 的滚动容器 inject key。 */
export const CHAT_SCROLL_EL_KEY = 'chat-messages-scroll-el';

export type LazyTaskHydrationOpts = {
  /** 消息滚动容器。 */
  scrollEl: Ref<HTMLElement | null>;
  /** 折叠卡判定：true = 该 task 未水合且未在水合中，需要自动水合。 */
  needsHydration: (taskId: string) => boolean;
  /** 水合触发（store action，幂等）。 */
  hydrate: (taskId: string) => Promise<void>;
};

export function useLazyTaskHydration(opts: LazyTaskHydrationOpts) {
  const collapsedIds = ref(new Set<string>());
  let observer: IntersectionObserver | null = null;
  const dwellTimers = new Map<string, ReturnType<typeof setTimeout>>();
  const observed = new Map<string, Element>();

  function cancelDwell(taskId: string) {
    const timer = dwellTimers.get(taskId);
    if (timer) {
      clearTimeout(timer);
      dwellTimers.delete(taskId);
    }
  }

  function handleEntries(entries: IntersectionObserverEntry[]) {
    for (const e of entries) {
      const el = e.target as HTMLElement;
      const taskId = el.dataset.taskId;
      if (!taskId) continue;
      if (e.isIntersecting) {
        if (!opts.needsHydration(taskId) || dwellTimers.has(taskId)) continue;
        dwellTimers.set(
          taskId,
          setTimeout(() => {
            dwellTimers.delete(taskId);
            void hydrateWithAnchor(taskId);
          }, HYDRATE_DWELL_MS),
        );
      } else {
        cancelDwell(taskId);
      }
    }
  }

  /** 同步观察集合：tasks 渲染/水合状态变化后由调用方 nextTick 触发。 */
  function syncCards() {
    const root = opts.scrollEl.value;
    if (!root) return;
    if (!observer && typeof IntersectionObserver !== 'undefined') {
      observer = new IntersectionObserver(handleEntries, { root, threshold: IO_THRESHOLD });
    }
    const seen = new Set<string>();
    for (const el of root.querySelectorAll<HTMLElement>('.task-card[data-task-id]')) {
      const taskId = el.dataset.taskId;
      if (!taskId) continue;
      seen.add(taskId);
      const watching = observed.has(taskId);
      if (opts.needsHydration(taskId)) {
        if (!watching) {
          observer?.observe(el);
          observed.set(taskId, el);
        }
      } else if (watching) {
        observer?.unobserve(observed.get(taskId)!);
        observed.delete(taskId);
        cancelDwell(taskId);
      }
    }
    // 清理已从 DOM 移除的卡片（会话切换等）。
    for (const [taskId, el] of [...observed]) {
      if (!seen.has(taskId)) {
        observer?.unobserve(el);
        observed.delete(taskId);
        cancelDwell(taskId);
      }
    }
  }

  /** 滚动锚定：仅当卡片原位置在视口上方时，按高度增量补偿 scrollTop。 */
  async function hydrateWithAnchor(taskId: string) {
    const el = opts.scrollEl.value;
    const card = el?.querySelector<HTMLElement>(`.task-card[data-task-id="${CSS.escape(taskId)}"]`);
    const wasAbove =
      !!el && !!card && card.getBoundingClientRect().top < el.getBoundingClientRect().top;
    const prevHeight = el?.scrollHeight ?? 0;
    await opts.hydrate(taskId);
    await nextTick();
    if (el && wasAbove) {
      el.scrollTop += el.scrollHeight - prevHeight;
    }
  }

  /** 点击卡片立即水合（无 dwell）。 */
  function expandTask(taskId: string) {
    void hydrateWithAnchor(taskId);
  }

  function isCollapsed(taskId: string): boolean {
    return collapsedIds.value.has(taskId);
  }

  function toggleCollapse(taskId: string) {
    if (collapsedIds.value.has(taskId)) collapsedIds.value.delete(taskId);
    else collapsedIds.value.add(taskId);
  }

  onBeforeUnmount(() => {
    observer?.disconnect();
    for (const t of dwellTimers.values()) clearTimeout(t);
    dwellTimers.clear();
    observed.clear();
  });

  return { collapsedIds, isCollapsed, toggleCollapse, expandTask, syncCards };
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd web && pnpm vitest run src/features/chat/composables/__tests__/useLazyTaskHydration.spec.ts`
Expected: 7 个用例全 PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/features/chat/composables/useLazyTaskHydration.ts web/src/features/chat/composables/__tests__/useLazyTaskHydration.spec.ts
git commit -m "feat(web): add useLazyTaskHydration (IntersectionObserver dwell + scroll anchor)"
```

---

### Task 10: useActivityQueries 门面 + i18n 键

**Files:**
- Modify: `web/src/features/chat/composables/useActivityQueries.ts`
- Modify: `web/src/i18n/locales/zh-CN.ts`（chat.v2 区段，L522 `resumeTaskSent` 之后）
- Modify: `web/src/i18n/locales/en-US.ts`（chat.v2 区段，L506 `resumeTaskSent` 之后）

- [ ] **Step 1: 门面加两个查询方法**

`useActivityQueries.ts` return 对象的 `teamStages()` 之前（`// --- Direct map access` 注释上方）插入：

```typescript
    // --- Lazy hydration (chat history lazy load) ---
    isTaskHydrated(taskId: string): boolean {
      return store.hydratedTaskIds.has(taskId);
    },
    taskHydrationState(taskId: string): 'loading' | 'error' | undefined {
      return store.taskHydration.get(taskId);
    },
```

- [ ] **Step 2: i18n 键**

`zh-CN.ts` 在 `resumeTaskSent: '已请求继续执行',` 之后追加：

```typescript
      // 历史轮次懒加载（折叠卡 meta-bar + 收起按钮）
      collapseExecution: '收起执行过程',
      loadFailedRetry: '加载失败，点击重试',
```

`en-US.ts` 在 `resumeTaskSent: 'Resume requested',` 之后追加：

```typescript
      // Lazy-loaded history cards (collapsed meta-bar + collapse button)
      collapseExecution: 'Collapse execution',
      loadFailedRetry: 'Load failed, click to retry',
```

- [ ] **Step 3: 校验**

Run: `cd web && pnpm lint`
Expected: exit 0

- [ ] **Step 4: Commit（与 Task 11 合并，见 Task 11 Step 5）**

---

### Task 11: TaskCard 四态改造 + 组件测试

**Files:**
- Modify: `web/src/components/chat/v2/TaskCard.vue`
- Create: `web/src/components/chat/v2/__tests__/TaskCard.spec.ts`

- [ ] **Step 1: 写失败测试**

新建 `TaskCard.spec.ts`：

```typescript
// web/src/components/chat/v2/__tests__/TaskCard.spec.ts
// 设计：docs/superpowers/specs/2026-07-23-chat-history-lazy-load-design.md §4.4
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import TaskCard from '../TaskCard.vue';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
import type { Task } from '../../../../features/chat/v2Types';

function mkTask(over: Partial<Task> = {}): Task {
  return {
    ID: 't-1',
    SessionID: 'sess-1',
    UserMessage: '帮我写一份季度总结',
    Status: 'completed',
    Seq: 1,
    Version: 1,
    CreatedAt: '2026-07-23T10:00:00Z',
    UpdatedAt: '2026-07-23T10:01:30Z',
    CompletedAt: '2026-07-23T10:01:30Z',
    ...over,
  };
}

describe('TaskCard lazy hydration states', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('collapsed (!hydrated): renders user panel + meta-bar, zero execution DOM', () => {
    const wrapper = mount(TaskCard, { props: { task: mkTask(), hydrated: false } });
    // 用户指令面板原样
    expect(wrapper.find('.task-user-panel__text').text()).toBe('帮我写一份季度总结');
    // meta-bar：状态徽章 + 耗时
    expect(wrapper.find('.task-meta-bar').exists()).toBe(true);
    expect(wrapper.find('.task-meta-bar__duration').text()).toContain('1m30s');
    // 零执行过程 DOM
    expect(wrapper.find('.turn-list').exists()).toBe(false);
    expect(wrapper.find('.task-card__collapse-btn').exists()).toBe(false);
  });

  it('collapsed card click emits hydrate', async () => {
    const wrapper = mount(TaskCard, { props: { task: mkTask(), hydrated: false } });
    await wrapper.find('.task-card').trigger('click');
    expect(wrapper.emitted('hydrate')?.length).toBe(1);
    expect(wrapper.emitted('hydrate')?.[0]).toEqual([mkTask()]);
  });

  it('action buttons do not bubble to card click (no hydrate on copy/regenerate)', async () => {
    const wrapper = mount(TaskCard, { props: { task: mkTask(), hydrated: false } });
    await wrapper.find('.task-user-panel__action-btn').trigger('click');
    expect(wrapper.emitted('hydrate')).toBeUndefined();
  });

  it('loading state renders shimmer skeleton instead of meta-bar', () => {
    const wrapper = mount(TaskCard, {
      props: { task: mkTask(), hydrated: false, hydrationState: 'loading' },
    });
    expect(wrapper.find('.task-card__skeleton').exists()).toBe(true);
    expect(wrapper.findAll('.task-card__skeleton-bar').length).toBe(3);
    expect(wrapper.find('.task-meta-bar').exists()).toBe(false);
  });

  it('error state meta-bar shows retry hint and re-emits hydrate on click', async () => {
    const wrapper = mount(TaskCard, {
      props: { task: mkTask(), hydrated: false, hydrationState: 'error' },
    });
    expect(wrapper.find('.task-meta-bar--error').exists()).toBe(true);
    await wrapper.find('.task-card').trigger('click');
    expect(wrapper.emitted('hydrate')?.length).toBe(1);
  });

  it('hydrated + !collapsed: full render + collapse button emits toggle-collapse', async () => {
    const store = useChatActivityStore();
    store.upsertTask(mkTask());
    const wrapper = mount(TaskCard, { props: { task: mkTask(), hydrated: true, collapsed: false } });
    expect(wrapper.find('.task-meta-bar').exists()).toBe(false);
    const btn = wrapper.find('.task-card__collapse-btn');
    expect(btn.exists()).toBe(true);
    await btn.trigger('click');
    expect(wrapper.emitted('toggle-collapse')?.length).toBe(1);
  });

  it('hydrated + collapsed: meta-bar again, click emits toggle-collapse (no refetch)', async () => {
    const wrapper = mount(TaskCard, { props: { task: mkTask(), hydrated: true, collapsed: true } });
    expect(wrapper.find('.task-meta-bar').exists()).toBe(true);
    await wrapper.find('.task-card').trigger('click');
    expect(wrapper.emitted('toggle-collapse')?.length).toBe(1);
    expect(wrapper.emitted('hydrate')).toBeUndefined();
  });
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd web && pnpm vitest run src/components/chat/v2/__tests__/TaskCard.spec.ts`
Expected: FAIL — `hydrated` prop 不存在 / `.task-meta-bar` 不存在

- [ ] **Step 3: 实现 TaskCard 四态**

`TaskCard.vue` 模板改造（完整替换 `<template>` 根 div 内容结构）：

```vue
<template>
  <div
    class="task-card"
    :class="{
      'task-card--clickable': !hydrated || collapsed,
    }"
    :data-task-id="task.ID"
    @click="onCardClick"
  >
    <!-- 用户消息统一面板：时间 + 头像 + 名称 + 消息内容 + 操作按钮（原样，按钮加 .stop） -->
    <div class="task-user-panel">
      <div class="task-user-panel__header">
        <span class="task-user-panel__time">{{ formattedTime }}</span>
        <div class="task-user-panel__avatar" :aria-label="userLabel">{{ avatarLetter }}</div>
        <span class="task-user-panel__name">{{ userLabel }}</span>
      </div>
      <div class="task-user-panel__body" :data-chat-user-prompt="task.UserMessage">
        <div class="task-user-panel__text">{{ task.UserMessage }}</div>
      </div>
      <div class="task-user-panel__actions">
        <q-btn
          flat
          dense
          round
          size="sm"
          :aria-label="t('chat.copy')"
          icon="content_copy"
          class="task-user-panel__action-btn"
          @click.stop="copyMessage"
        >
          <q-tooltip>{{ t('chat.copy') }}</q-tooltip>
        </q-btn>
        <q-btn
          flat
          dense
          round
          size="sm"
          :aria-label="t('chat.regenerate')"
          icon="refresh"
          class="task-user-panel__action-btn"
          @click.stop="$emit('regenerate', task)"
        >
          <q-tooltip>{{ t('chat.regenerate') }}</q-tooltip>
        </q-btn>
      </div>
    </div>

    <!-- 水合中：用户面板 + 3 条 shimmer 骨架 -->
    <div v-if="!hydrated && hydrationState === 'loading'" class="task-card__skeleton" aria-hidden="true">
      <div class="task-card__skeleton-bar" style="width: 62%" />
      <div class="task-card__skeleton-bar" style="width: 38%" />
      <div class="task-card__skeleton-bar" style="width: 81%" />
    </div>

    <!-- 折叠态（未水合 / 水合后手动收起）：slim meta-bar -->
    <div
      v-else-if="!hydrated || collapsed"
      class="task-meta-bar"
      :class="[`task-meta-bar--${statusTone}`, { 'task-meta-bar--error': hydrationState === 'error' }]"
    >
      <span class="task-meta-bar__badge">{{ statusLabel }}</span>
      <span v-if="durationText" class="task-meta-bar__duration">⏱ {{ durationText }}</span>
      <span v-if="hydrationState === 'error'" class="task-meta-bar__error-text">
        {{ t('chat.v2.loadFailedRetry') }}
      </span>
    </div>

    <!-- 水合态：现状完整渲染 + 底部收起按钮 -->
    <template v-else>
      <div v-if="task.Status === 'running'" class="task-status">{{ t('chat.v2.taskProcessing') }}</div>
      <!-- 澄清门卡片：orphan step（TurnID 空，澄清在 Run/Turn 创建前发布） -->
      <ClarifyBlock
        v-for="s in orphanClarifySteps"
        :key="s.ID"
        :step="s"
        @submit-clarification="(p) => $emit('submit-clarification', p)"
      />
      <!-- L3: 中断任务入口 — 服务重启导致的中断，点击「继续执行」触发 WS resume_task -->
      <div v-if="task.Status === 'interrupted'" class="task-interrupted">
        <q-icon name="pause_circle_outline" size="16px" class="task-interrupted__icon" />
        <span class="task-interrupted__label">{{ t('chat.v2.taskInterrupted') }}</span>
        <q-btn
          unelevated
          dense
          no-caps
          size="sm"
          color="accent"
          class="task-interrupted__btn"
          :label="t('chat.v2.resumeTask')"
          @click.stop="$emit('resume-task', task)"
        />
      </div>
      <TurnList
        v-if="prePlanTurns.length"
        :turns="prePlanTurns"
        @pause-agent="(sid) => $emit('pause-agent', sid)"
        @inject-agent="(p) => $emit('inject-agent', p)"
        @retry-team="(teamId) => $emit('retry-team', teamId)"
        @expand="(ids) => $emit('expand', ids)"
        @confirm-step="(p) => $emit('confirm-step', p)"
      />
      <template v-for="pb in planBoards" :key="pb.ID">
        <PlanBoardCard :plan-board="pb" />
        <GraphStageBlock v-if="graphStageByPlanBoard(pb.ID)" :graph-stage="graphStageByPlanBoard(pb.ID)!" />
      </template>
      <TeamStagePanel
        v-for="ts in orphanTeamStages"
        :key="ts.ID"
        :team-stage="ts"
        @pause-agent="(sid) => $emit('pause-agent', sid)"
        @inject-agent="(p) => $emit('inject-agent', p)"
        @retry-team="(teamId) => $emit('retry-team', teamId)"
        @expand="(ids) => $emit('expand', ids)"
        @confirm-step="(p) => $emit('confirm-step', p)"
      />
      <MemberSessionPanel
        v-for="ms in orphanMemberSessions"
        :key="ms.ID"
        :member-session="ms"
        @pause-agent="(sid) => $emit('pause-agent', sid)"
        @inject-agent="(p) => $emit('inject-agent', p)"
        @expand="(ids) => $emit('expand', ids)"
        @confirm-step="(p) => $emit('confirm-step', p)"
      />
      <TurnList
        v-if="postPlanTurns.length"
        :turns="postPlanTurns"
        @pause-agent="(sid) => $emit('pause-agent', sid)"
        @inject-agent="(p) => $emit('inject-agent', p)"
        @retry-team="(teamId) => $emit('retry-team', teamId)"
        @expand="(ids) => $emit('expand', ids)"
        @confirm-step="(p) => $emit('confirm-step', p)"
      />
      <NoticeBlock v-for="s in orphanNoticeSteps" :key="s.ID" :step="s" />
      <button class="task-card__collapse-btn" type="button" @click.stop="$emit('toggle-collapse', task)">
        {{ t('chat.v2.collapseExecution') }} ▴
      </button>
    </template>
  </div>
</template>
```

script 改造：

```typescript
const props = withDefaults(
  defineProps<{
    task: Task;
    /** 执行子树是否已水合（false = 折叠卡）。默认 true 兼容既有用法。 */
    hydrated?: boolean;
    /** 水合进行/失败态（仅 !hydrated 时有意义）。 */
    hydrationState?: 'loading' | 'error';
    /** 水合后手动收起（UI 态，数据保留 store，再展开零请求）。 */
    collapsed?: boolean;
  }>(),
  { hydrated: true, hydrationState: undefined, collapsed: false },
);
defineEmits<{
  regenerate: [task: Task];
  'resume-task': [task: Task];
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  'retry-team': [teamId: string];
  expand: [sessionIds: string[]];
  'confirm-step': [payload: ConfirmStepPayload];
  'submit-clarification': [payload: SubmitClarificationPayload];
  hydrate: [task: Task];
  'toggle-collapse': [task: Task];
}>();
```

script 追加 computed/handler：

```typescript
/** 折叠卡整卡点击：未水合 → 请求水合；水合后收起态 → 本地展开（零请求）。 */
function onCardClick() {
  if (!props.hydrated) emit('hydrate', props.task);
  else if (props.collapsed) emit('toggle-collapse', props.task);
}
```

（注意：`defineEmits` 改为 `const emit = defineEmits<{...}>()`，原有模板内 `$emit` 调用保持不变。）

```typescript
/** meta-bar 状态徽章文案：复用 chat.v2.status* 既有键。 */
const statusLabel = computed(() => {
  const key = `chat.v2.status${props.task.Status.charAt(0).toUpperCase()}${props.task.Status.slice(1)}`;
  return t(key);
});

/** meta-bar 状态色调：completed=success / failed=danger / 其余=neutral。 */
const statusTone = computed(() => {
  if (props.task.Status === 'completed') return 'success';
  if (props.task.Status === 'failed') return 'danger';
  return 'neutral';
});

/** 耗时文案：CompletedAt - CreatedAt → "Ns" / "NmSSs"；未完结不显示。 */
const durationText = computed(() => {
  if (!props.task.CompletedAt) return '';
  const ms = Date.parse(props.task.CompletedAt) - Date.parse(props.task.CreatedAt);
  if (!Number.isFinite(ms) || ms < 0) return '';
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s}s`;
  return `${Math.floor(s / 60)}m${String(s % 60).padStart(2, '0')}s`;
});
```

style 追加（`<style lang="sass" scoped>` 内末尾）：

```sass
/* 折叠卡整卡可点（复制/重新生成按钮 @click.stop 不触发） */
.task-card--clickable
  cursor: pointer

  .task-user-panel__body
    transition: border-color 0.2s, background 0.2s

  &:hover .task-user-panel__body
    border-color: var(--color-accent)
    background: color-mix(in srgb, var(--glass-surface) 80%, var(--color-accent) 8%)

/* slim meta-bar：状态徽章 + 耗时（color-mix 状态色，日夜 token） */
.task-meta-bar
  display: flex
  align-items: center
  gap: 8px
  margin: -2px 0 8px auto
  width: fit-content
  padding: 4px 10px
  border-radius: 999px
  font-size: 12px
  border: 1px solid var(--glass-border)
  background: var(--glass-surface)

  &__badge
    font-weight: 500

  &__duration
    color: var(--color-text-tertiary)
    font-variant-numeric: tabular-nums

  &--success &__badge
    color: var(--color-success, #2e7d32)

  &--danger &__badge
    color: var(--color-danger, #d32f2f)

  &--neutral &__badge
    color: var(--color-text-secondary)

  &--error
    border-color: var(--color-danger, #d32f2f)

  &__error-text
    color: var(--color-danger, #d32f2f)

/* 水合中 shimmer 骨架（thinking/action/reply 三条） */
.task-card__skeleton
  display: flex
  flex-direction: column
  gap: 10px
  margin: 4px 0 8px

  &-bar
    height: 14px
    border-radius: 7px
    background: linear-gradient(90deg, var(--glass-surface) 25%, var(--glass-border) 50%, var(--glass-surface) 75%)
    background-size: 200% 100%
    animation: task-card-shimmer 1.4s infinite

@keyframes task-card-shimmer
  0%
    background-position: 200% 0
  100%
    background-position: -200% 0

/* 收起执行过程按钮（水合态底部） */
.task-card__collapse-btn
  display: block
  margin: 4px auto 8px
  padding: 4px 12px
  border: none
  background: transparent
  color: var(--color-text-tertiary)
  font-size: 12px
  cursor: pointer
  border-radius: 6px
  transition: color 0.2s, background 0.2s

  &:hover
    color: var(--color-accent)
    background: color-mix(in srgb, var(--color-accent) 8%, transparent)
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd web && pnpm vitest run src/components/chat/v2/__tests__/TaskCard.spec.ts src/components/chat/v2/__tests__/ClarifyBlock.spec.ts`
Expected: 新用例全 PASS；ClarifyBlock 既有用例不回归（hydrated 默认 true 走原渲染路径）

- [ ] **Step 5: Commit（与 Task 10/12 合并，见 Task 12 Step 5）**

---

### Task 12: TaskList 接线 + ChatMessageList provide

**Files:**
- Modify: `web/src/components/chat/v2/TaskList.vue`
- Modify: `web/src/components/chat/ChatMessageList.vue:58-69`

**前置**: Task 9/10/11 完成。

- [ ] **Step 1: ChatMessageList provide 滚动容器**

`ChatMessageList.vue` script 中，在 `provide` 已 import 的前提下，于 props/emit 定义后追加（约 L120 之后）：

```typescript
import { CHAT_SCROLL_EL_KEY } from '../../features/chat/composables/useLazyTaskHydration';

// 懒水合 composable 的 observer root（折叠卡视口感知）。
provide(CHAT_SCROLL_EL_KEY, scrollViewportEl);
```

（`scrollViewportEl` 是 L31 模板 ref；确认其声明名不变。若该文件已有同名 import 或 provide，去重。）

- [ ] **Step 2: TaskList 接入 composable**

`TaskList.vue` 完整替换为：

```vue
<!-- web/src/components/chat/v2/TaskList.vue -->
<template>
  <div class="task-list">
    <TaskCard
      v-for="task in tasks"
      :key="task.ID"
      :task="task"
      :hydrated="queries.isTaskHydrated(task.ID)"
      :hydration-state="queries.taskHydrationState(task.ID)"
      :collapsed="lazy.isCollapsed(task.ID)"
      @regenerate="(t) => $emit('regenerate', t)"
      @resume-task="(t) => $emit('resume-task', t)"
      @pause-agent="(sid) => $emit('pause-agent', sid)"
      @inject-agent="(p) => $emit('inject-agent', p)"
      @retry-team="(teamId) => $emit('retry-team', teamId)"
      @expand="(ids) => $emit('expand', ids)"
      @confirm-step="(p) => $emit('confirm-step', p)"
      @submit-clarification="(p) => $emit('submit-clarification', p)"
      @hydrate="(t) => lazy.expandTask(t.ID)"
      @toggle-collapse="(t) => lazy.toggleCollapse(t.ID)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, inject, nextTick, watch, type Ref } from 'vue';
import { useActivityQueries } from '../../../features/chat/composables/useActivityQueries';
import {
  useLazyTaskHydration,
  CHAT_SCROLL_EL_KEY,
} from '../../../features/chat/composables/useLazyTaskHydration';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { Task } from '../../../features/chat/v2Types';
import type { ConfirmStepPayload, SubmitClarificationPayload } from '../../../features/chat/types';
import TaskCard from './TaskCard.vue';

const props = defineProps<{ sessionId: string }>();
defineEmits<{
  regenerate: [task: Task];
  'resume-task': [task: Task];
  'pause-agent': [sessionId: string];
  'inject-agent': [payload: { sessionId: string; message: string }];
  'retry-team': [teamId: string];
  expand: [sessionIds: string[]];
  'confirm-step': [payload: ConfirmStepPayload];
  'submit-clarification': [payload: SubmitClarificationPayload];
}>();
const queries = useActivityQueries();
const tasks = computed(() => queries.getSessionTasks(props.sessionId));

// 懒水合编排：折叠卡滚入视口 500ms / 点击 → store.hydrateTask。
// composable 在 features 层，允许直访 store（组件层边界合规）。
const store = useChatActivityStore();
const scrollEl = inject<Ref<HTMLElement | null>>(CHAT_SCROLL_EL_KEY, computed(() => null) as never);
const lazy = useLazyTaskHydration({
  scrollEl,
  needsHydration: (taskId) => !store.hydratedTaskIds.has(taskId) && store.taskHydration.get(taskId) !== 'loading',
  hydrate: (taskId) => store.hydrateTask(taskId),
});

// tasks 渲染 / 水合状态变化后同步观察集合（flush: 'post' 保证 DOM 已更新）。
watch(
  () => tasks.value.map((t) => `${t.ID}:${store.hydratedTaskIds.has(t.ID)}`).join('|'),
  async () => {
    await nextTick();
    lazy.syncCards();
  },
  { immediate: true, flush: 'post' },
);
</script>
```

注意：本组件直接 import store 仅为向 composable 传参——检查 `scripts/check-frontend-layer.mjs` 是否禁止 components 引入 stores；若禁止，将 `needsHydration`/`hydrate` 改用 `useActivityQueries` 门面 + 新增 `hydrateTask` 门面包装（在 Task 10 的门面中补 `hydrateTask(taskId)` 方法转发 store action，并从此组件移除 store import）。**实施时先跑 `pnpm lint` 判定，违规则走门面方案。**

- [ ] **Step 3: 类型/lint 校验**

Run: `cd web && pnpm lint`
Expected: exit 0（若 layer 检查报 store import，按上门面方案调整）

- [ ] **Step 4: 全量前端测试**

Run: `cd web && pnpm test`
Expected: 全 PASS（重点观察 TaskList/SessionPanel 相关既有用例；若既有 TaskCard 挂载用例未传 hydrated prop，默认 true 兼容）

- [ ] **Step 5: Commit（含 Task 10/11）**

```bash
git add web/src/features/chat/composables/useActivityQueries.ts web/src/i18n/locales/ web/src/components/chat/v2/TaskCard.vue web/src/components/chat/v2/__tests__/TaskCard.spec.ts web/src/components/chat/v2/TaskList.vue web/src/components/chat/ChatMessageList.vue
git commit -m "feat(web): TaskCard collapsed meta-bar states + TaskList lazy hydration wiring"
```

---

## 验证与文档任务

### Task 13: 全量验证

**Files:** 无（仅运行命令）

- [ ] **Step 1: 后端全量**

Run: `go build ./... && go test ./internal/service/... ./internal/biz/... ./internal/data/... -count=1`
Expected: exit 0，全 PASS（data PG 用例允许 SKIP；`internal/agent` 未改动可跳过，若时间允许跑 `go test ./internal/...`）

- [ ] **Step 2: 前端全量**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: exit 0，全 PASS

- [ ] **Step 3: 运行时手测（R3 规则：修复后必须运行时验证）**

1. 启动 admin（:8000）+ 前端（:9001），打开一个含多轮历史的会话
2. DevTools Network：首屏仅见 `tasks` + `steps?limit=100` + 少量 per-task 请求（最后一个 task + 非终态）
3. 页面直接停在消息底部；历史卡显示折叠 meta-bar
4. 向上滚动：折叠卡停留 500ms 自动展开；展开时视口不跳动
5. 点击折叠卡立即展开；点「收起执行过程」重新折叠；再展开无网络请求
6. 中断任务卡直接可见「继续执行」按钮

### Task 14: 文档同步（DOC-SYNC 红线）

**Files:**
- Modify: `docs/development/1-chat.md`
- Modify: `docs/development/1-chat.design.md`
- Modify: `docs/development/1-chat.development.md`

> 操作前必读 `aranea-docs-guide` SKILL。

- [ ] **Step 1: 需求文档 `1-chat.md`**

追加需求段落「长会话历史懒加载」（仅用户视角行为 P1-P6 + 验收标准，不含实现细节）：

```markdown
### 长会话历史懒加载（2026-07-23）

**用户故事**：作为用户，打开有几十上百轮历史的会话时，希望立即可读可输入，而不是等待全部执行过程加载完。

**功能需求**：
- P1：我的全部历史指令一次性可见（显示方式与现状一致）
- P2：最新一轮的执行过程默认展开；进行中的任务（运行中/等待中/已中断）默认展开
- P3：更早轮次的执行过程折叠为一行状态摘要（状态+耗时）；点击卡片或滚动停留片刻自动展开
- P4：打开会话后直接停在最新消息位置；向上翻看历史时页面不跳动
- P5：已中断的任务直接显示「继续执行」入口
- P6：展开的历史轮次可以手动收起；再次展开无需重新加载

**验收标准**：
1. 打开 200 轮会话：首屏网络请求数 ≈ 2 + 5×进行中任务数，页面秒级可交互
2. 打开后视口停在消息底部，无白屏等待
3. 折叠卡进入视口停留约半秒自动展开，点击立即展开，展开时视口不跳动
4. 会话进行中收到新消息/新任务时实时渲染不受影响
```

- [ ] **Step 2: 设计文档 `1-chat.design.md`**

追加设计段落（ListSteps 分页契约 / 分阶段水合流程 / TaskCard 状态机 / useLazyTaskHydration 设计），内容以 `docs/superpowers/specs/2026-07-23-chat-history-lazy-load-design.md` §4 为准压缩转写（架构/契约/状态机视角，不含任务清单）。

- [ ] **Step 3: 开发计划 `1-chat.development.md`**

追加任务清单（本计划 14 个任务的压缩版 + 状态 ✅）与代码锚点：

```markdown
| 锚点 | 文件 |
|------|------|
| ListStepsV2 分页契约 | `api/kratos/session/v1/session.proto` ListStepsV2Request/Response |
| 分页 repo | `internal/data/step_v2_repo.go` ListStepsBySessionPaged |
| 索引迁移 | `internal/data/sql/migrations/20261109_steps_v2_session_seq.sql`（registry 20261109） |
| service 分发 | `internal/service/session_v2.go` ListSteps |
| 分阶段水合 + hydrateTask | `web/src/stores/chat/activityV2Store.ts` fetchSessionHistory / hydrateTask |
| 懒水合编排 | `web/src/features/chat/composables/useLazyTaskHydration.ts` |
| 折叠卡四态 | `web/src/components/chat/v2/TaskCard.vue` |
| 接线 | `web/src/components/chat/v2/TaskList.vue`、`web/src/components/chat/ChatMessageList.vue` |
```

- [ ] **Step 4: Commit**

```bash
git add docs/development/1-chat.md docs/development/1-chat.design.md docs/development/1-chat.development.md
git commit -m "docs(chat): sync lazy hydration spec into 1-chat three-piece docs"
```

---

## 边界情况核对表（实施时逐项确认）

| 场景 | 预期 | 覆盖位置 |
|------|------|---------|
| WS 重连 | hydratedTaskIds 不清空，已展开卡保持展开 | Task 8 store 测试 |
| 会话中新建 task | task.created → hydratedTaskIds.add → 默认展开 | Task 7 router 测试 |
| interrupted task | 非终态 → 自动水合 → 「继续执行」直接可见 | Task 8 测试 + Task 11 组件 |
| 无 step 的 task（纯澄清） | 折叠卡正常显示徽章；水合后渲染澄清卡 | Task 11 组件（ClarifyBlock 回归） |
| 水合失败 | meta-bar「加载失败，点击重试」；点击重试 | Task 8 store + Task 11 组件 |
| 单 task 超大 | 协议分页已就位；UI 分页不做（YAGNI） | 设计 §5，无代码 |
| spirit 级 orphan steps | Phase 1 limit=100 窗口覆盖；窗口不够再议（YAGNI） | Task 8 |
| 后端 repo 错误 | 统一 `entErrToBizErr` 翻译（DB-R5） | Task 3 实现 |

## 自查清单（plan self-review 已执行）

- [x] Spec 覆盖：§4.1 proto/repo/索引 → Task 1-5；§4.2 store → Task 7-8；§4.3 composable → Task 9；§4.4 组件 → Task 10-12；§5 边界 → 核对表；§6 错误处理 → Task 5/8/11；§7 测试策略 → 各 Task TDD 步骤；§9 文档 → Task 14
- [x] 无占位符：所有代码步骤含完整代码
- [x] 类型一致：`hydrateTask(taskId: string): Promise<void>`（store）= composable opts.hydrate 签名；`hydratedTaskIds: Ref<Set<string>>` / `taskHydration: Ref<Map<string,'loading'|'error'>>` 全链一致；TaskCard props `hydrated/hydrationState/collapsed` 与 TaskList 传参一致；i18n 键 `chat.v2.collapseExecution` / `chat.v2.loadFailedRetry` 与组件引用一致
