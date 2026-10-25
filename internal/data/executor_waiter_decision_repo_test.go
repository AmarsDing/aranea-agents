package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// executor_waiter_decision_repo_test.go —— D6（97-multi-replica-executor-
// waiters）PG 实现语义钉死套件。跨副本验收的 service 侧机制（轮询消费/
// ctx 死亡终查/绝不误杀）已由 InMem 端口等价物钉板
//（plan_executor_waiter_d6_test.go）——端口语义等价性是 D6 灰度前提，
// 本文件只钉 PG 独有语义：真实 ON CONFLICT CAS、定宽时间往返、
// 双 repo 实例（=两副本连接）先到者赢。
//
// 调度纪律：无全表计数断言，用例间按自有 scope_id 隔离，全部可并行。
// PG 容器（55432）缺席时与存量 PG 钉同族失败（G-37 归口，容器修复后
// 裸跑补绿——无 integration tag）。

// ensureExecutorWaiterDecisionsTable 逐句镜像迁移
// 20261302_executor_waiter_decisions.sql（raw-SQL 表不经 ent
// Schema.Create；迁移文件改动必须同步本镜像，列序钉死测试依赖不漂移）。
func ensureExecutorWaiterDecisionsTable(t *testing.T, d *Data) {
	t.Helper()
	ctx := context.Background()
	db := d.RWDB().WriteDB(ctx)
	if db == nil {
		t.Fatal("test data rwdb unavailable")
	}
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS executor_waiter_decisions (
  kind       TEXT    NOT NULL,
  session_id TEXT    NOT NULL DEFAULT '',
  scope_id   TEXT    NOT NULL,
  decision   BOOLEAN NOT NULL DEFAULT FALSE,
  reason     TEXT    NOT NULL DEFAULT '',
  actor      TEXT    NOT NULL DEFAULT '',
  decided_at TEXT    NOT NULL DEFAULT '',
  PRIMARY KEY (kind, session_id, scope_id)
);
`)
	if err != nil {
		t.Fatalf("create executor_waiter_decisions mirror DDL: %v", err)
	}
}

// TestExecutorWaiterDecisionRepo_CASFirstWriterWins 跨副本核心钉：两个
// repo 实例（共享一表=两副本连接面）并发语义——B 副本 TryRecord 命中
// 冲突 recorded=false，LoadDecision 读回 A 副本先到决议（严禁翻案）。
func TestExecutorWaiterDecisionRepo_CASFirstWriterWins(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	ensureExecutorWaiterDecisionsTable(t, d)
	replicaA := NewExecutorWaiterDecisionRepo(d)
	replicaB := NewExecutorWaiterDecisionRepo(d)
	ctx := context.Background()
	key := biz.ExecutorWaiterDecisionKey{
		Kind:      biz.WaiterDecisionKindPlaybookConfirm,
		SessionID: "sp-cas",
		ScopeID:   "st-cas",
	}

	recorded, err := replicaA.TryRecordDecision(ctx, biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: key,
		Approved:                  true,
		Reason:                    "approve",
		Actor:                     "replica-a",
		DecidedAt:                 time.Now().UTC(),
	})
	if err != nil || !recorded {
		t.Fatalf("replica A first write: recorded=%v err=%v", recorded, err)
	}
	// B 副本同 key 冲突写（反向决议）：必须输掉 CAS 且不得翻案。
	recorded, err = replicaB.TryRecordDecision(ctx, biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: key,
		Approved:                  false,
		Reason:                    "deny",
		Actor:                     "replica-b",
		DecidedAt:                 time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("replica B conflict write: err=%v", err)
	}
	if recorded {
		t.Fatal("replica B conflict write must lose CAS (recorded=false)")
	}
	dec, found, err := replicaB.LoadDecision(ctx, key)
	if err != nil || !found {
		t.Fatalf("replica B load after conflict: found=%v err=%v", found, err)
	}
	if !dec.Approved || dec.Actor != "replica-a" || dec.Reason != "approve" {
		t.Fatalf("first-writer-wins violated: dec=%+v", dec)
	}
}

// TestExecutorWaiterDecisionRepo_Roundtrip 定宽时间布局往返（纳秒恒 9 位
// 钉死——board_run_leases 教训同律）+ reason/actor/空 session_id 明列往返
// + 零值 DecidedAt 由实现侧补戳。
func TestExecutorWaiterDecisionRepo_Roundtrip(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	ensureExecutorWaiterDecisionsTable(t, d)
	repo := NewExecutorWaiterDecisionRepo(d)
	ctx := context.Background()
	fixed := time.Unix(1_800_000_000, 123456789).UTC()
	key := biz.ExecutorWaiterDecisionKey{
		Kind:      biz.WaiterDecisionKindTeamAgentLoads,
		SessionID: "", // team 槽位快照按 team 寻址，session 可空
		ScopeID:   "team-rt",
	}

	if _, err := repo.TryRecordDecision(ctx, biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: key,
		Approved:                  true,
		Reason:                    "agent-a,agent-b",
		Actor:                     "plan_executor.dispatch",
		DecidedAt:                 fixed,
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	dec, found, err := repo.LoadDecision(ctx, key)
	if err != nil || !found {
		t.Fatalf("load: found=%v err=%v", found, err)
	}
	if !dec.DecidedAt.Equal(fixed) {
		t.Fatalf("decided_at 定宽往返失真: got %v want %v", dec.DecidedAt, fixed)
	}
	if dec.Reason != "agent-a,agent-b" || dec.Actor != "plan_executor.dispatch" || !dec.Approved {
		t.Fatalf("field roundtrip wrong: %+v", dec)
	}

	// 未决议 key：found=false + nil err（调用方据此区分「无决议」与「读失败」）。
	if _, found, err := repo.LoadDecision(ctx, biz.ExecutorWaiterDecisionKey{
		Kind: biz.WaiterDecisionKindBoardStartup, ScopeID: "pb-miss",
	}); err != nil || found {
		t.Fatalf("miss load: found=%v err=%v", found, err)
	}
}

// TestExecutorWaiterDecisionRepo_ValidationAndNilShell 写入期校验
// （kind/scope 必填）+ nil-data 壳两法一律 Internal（装配面 fail-closed）。
// D6-f2（C6）：写/读两径的 missing-scope_id 臂均探针（此前读径仅探
// missing-kind，删 `|| key.ScopeID == ""` 守卫整套仍绿的假绿区）。
func TestExecutorWaiterDecisionRepo_ValidationAndNilShell(t *testing.T) {
	t.Parallel()
	d := openTestData(t)
	ensureExecutorWaiterDecisionsTable(t, d)
	repo := NewExecutorWaiterDecisionRepo(d)
	ctx := context.Background()

	if _, err := repo.TryRecordDecision(ctx, biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{Kind: "", ScopeID: "x"},
	}); err == nil {
		t.Fatal("missing kind must be rejected")
	}
	if _, err := repo.TryRecordDecision(ctx, biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{Kind: "k", ScopeID: ""},
	}); err == nil {
		t.Fatal("write with missing scope must be rejected")
	}
	if _, _, err := repo.LoadDecision(ctx, biz.ExecutorWaiterDecisionKey{
		Kind: "", ScopeID: "x",
	}); err == nil {
		t.Fatal("load with missing kind must be rejected")
	}
	if _, _, err := repo.LoadDecision(ctx, biz.ExecutorWaiterDecisionKey{
		Kind: "k", ScopeID: "",
	}); err == nil {
		t.Fatal("load with missing scope must be rejected")
	}

	shell := NewExecutorWaiterDecisionRepo(nil)
	if _, err := shell.TryRecordDecision(ctx, biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{Kind: "k", ScopeID: "s"},
	}); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("nil shell write must be Internal, got %v", err)
	}
	if _, _, err := shell.LoadDecision(ctx, biz.ExecutorWaiterDecisionKey{Kind: "k", ScopeID: "s"}); !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("nil shell read must be Internal, got %v", err)
	}
}
