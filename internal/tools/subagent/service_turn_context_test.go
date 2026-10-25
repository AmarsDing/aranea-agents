package subagent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcsubagent "trpc.group/trpc-go/trpc-agent-go/openclaw/subagent"
)

// stubTagRunner 与 service_test.go 的 stubRunner 同构，带身份标签以区分
// 不同 turn 注入的 runner 实例（E2E-H4 capture 钉）。
type stubTagRunner struct {
	stubRunner
	tag string
}

// E2E-H4 pin ①：Spawn 把【父会话当前 turn】的 runner 快照进 runRecord；
// 之后同会话下一轮 turn（B）或另一会话（C）的注入都不得改写已捕获的
// runner——旧服务级单例末写者胜的错身份执行由此根除。
func TestSpawnCapturesSpawningTurnRunner(t *testing.T) {
	svc, err := NewService(t.TempDir(), nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.Start(context.Background())
	defer func() { _ = svc.Close() }()

	runnerA := &stubTagRunner{tag: "turn-A"}
	runnerB := &stubTagRunner{tag: "turn-B-next"}
	runnerC := &stubTagRunner{tag: "session-C"}
	svc.SetTurnContext("sess-a", runnerA, "prov-a", "mod-a", "agent-a", "key-a")

	run, err := svc.Spawn(context.Background(), SpawnRequest{
		OwnerUserID:     "u1",
		ParentSessionID: "sess-a",
		Task:            "task-a",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	svc.mu.Lock()
	rec := svc.runs[run.ID]
	svc.mu.Unlock()
	if rec == nil || rec.runner != runnerA {
		t.Fatalf("record must capture the spawning turn's runner, got %+v (rec=%v)", rec, rec != nil)
	}
	if rec.Attribution.AgentID != "agent-a" {
		t.Fatalf("record must capture the spawning turn's attribution, got %+v", rec.Attribution)
	}

	// 同会话下一轮换 runner + 另一会话注入：已捕获记录免疫。
	svc.SetTurnContext("sess-a", runnerB, "prov-b", "mod-b", "agent-b", "key-b")
	svc.SetTurnContext("sess-b", runnerC, "prov-c", "mod-c", "agent-c", "key-c")
	if rec.runner != runnerA {
		t.Fatal("captured runner mutated by a later turn's SetTurnContext (singleton semantics survived)")
	}
	if rec.Attribution.AgentID != "agent-a" {
		t.Fatalf("captured attribution mutated by a later turn: %+v", rec.Attribution)
	}

	// 另一会话 spawn 取自己的 turn runner（defaultRunner=nil 时也不得漏到 sess-a 的）。
	runB, err := svc.Spawn(context.Background(), SpawnRequest{
		OwnerUserID:     "u1",
		ParentSessionID: "sess-b",
		Task:            "task-b",
	})
	if err != nil {
		t.Fatalf("Spawn sess-b: %v", err)
	}
	svc.mu.Lock()
	recB := svc.runs[runB.ID]
	svc.mu.Unlock()
	if recB == nil || recB.runner != runnerC {
		t.Fatal("sess-b must execute on its OWN session's runner")
	}
}

// E2E-H4 pin ②：未捕获 runner 的记录（重启加载路径理论上不应到达执行）
// 必须 fail-loud，而不是解引用 nil interface panic。
func TestRunChildWithoutCapturedRunnerFails(t *testing.T) {
	svc, err := NewService(t.TempDir(), nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	rec := &runRecord{
		Run:         trpcsubagent.Run{ID: "r-no-runner", Task: "t"},
		OwnerUserID: "u1",
	}
	result := replyAccumulator{}
	usage := &usageAccum{}
	guard := &runBudgetGuard{svc: svc, runID: rec.ID, parentSID: "s1"}
	err = svc.runChild(context.Background(), rec, runningRun{}, &result, usage, guard)
	if err == nil || !strings.Contains(err.Error(), "no captured runner") {
		t.Fatalf("runChild without captured runner must fail loud, got %v", err)
	}
}

// E2E-H4 pin ③：durable 往返——captured runner 写入 json:"-"，持久化与
// 重载不得携带运行时引用（重载记录经 normalizeLoadedRuns 全部 failed，
// 永不进 runChild，此处 pin 序列化形状防回归）。
func TestRunRecordPersistRoundTripDropsRunner(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc.SetTurnContext("sess-a", &stubTagRunner{tag: "turn-A"}, "prov", "mod", "agent", "key")
	run, err := func() (trpcsubagent.Run, error) {
		svc.Start(context.Background())
		defer func() { _ = svc.Close() }()
		return svc.Spawn(context.Background(), SpawnRequest{
			OwnerUserID:     "u1",
			ParentSessionID: "sess-a",
			Task:            "task-a",
		})
	}()
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	svc2, err := NewService(dir, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	svc2.mu.Lock()
	reloaded := svc2.runs[run.ID]
	svc2.mu.Unlock()
	if reloaded == nil {
		t.Fatal("run record must persist across reload")
	}
	if reloaded.runner != nil {
		t.Fatal("reloaded record must NOT carry a runner reference (json:\"-\")")
	}
	// 非终态记录在加载时已被 normalizeLoadedRuns 落 failed——不可能带 nil
	// runner 进执行路径（pin ①/② 的兜底闭环）。
	if !reloaded.Status.IsTerminal() {
		t.Fatalf("reloaded non-terminal run must be normalized to terminal, got %q", reloaded.Status)
	}
}
