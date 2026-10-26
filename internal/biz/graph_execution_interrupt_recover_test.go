package biz

import (
	"context"
	"errors"
	"testing"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// E2E-H1 pins：生产不开 StreamModeCheckpoints 时，Pregel interrupt 事件是
// DomainEventGraphInterrupt 的唯一载体；其被背压丢弃后流直接结束，
// converge 必须先查持久化 interrupt checkpoint（source=interrupt）兜底收
// 敛 waiting_human——FSM failed→resume 非法，按 failed 收敛 = 永久不可恢复。

// interruptProbeRuntime 配置化的 GraphRuntime fake：ListCheckpoints 返回
// 预设页（exec.runtime 直连，绕过 factory）。
type interruptProbeRuntime struct {
	fakeRecoverRuntime
	list GraphCheckpointList
	err  error
}

func (r *interruptProbeRuntime) ListCheckpoints(context.Context, string, string, int) (GraphCheckpointList, error) {
	return r.list, r.err
}

func convergeInterruptExec(id string, rt GraphRuntime) *GraphExecution {
	exec := runningRecoverExec(id)
	exec.runtime = rt
	return exec
}

// pin①：interrupt checkpoint 存在 + 流无 done 结束 ⇒ 收敛 waiting_human
// （不置 FinishedAt / 不发失败外发），resume 保持合法。
func TestConvergeStreamEnd_DroppedInterruptRecoveredToWaitingHuman(t *testing.T) {
	exec := convergeInterruptExec("exec-h1", &interruptProbeRuntime{
		list: GraphCheckpointList{{
			Ref:    GraphCheckpointRef{LineageID: "lin-1", CheckpointID: "cp-1"},
			Source: graphCheckpointSourceInterrupt,
			Step:   3,
		}},
	})
	repo := &memGraphRunRepo{runs: map[string]*GraphExecution{exec.ID: exec}}
	uc := newRecoverTestUsecase(repo, nil, nil, exec.ID)

	uc.convergeStreamEnd(exec, exec.streamGen, exec.ID, exec.GraphID, false, nil)

	if exec.Status != string(GraphExecWaitingHuman) {
		t.Fatalf("status=%q, want waiting_human (durable interrupt checkpoint must override fail-closed)", exec.Status)
	}
	exec.interruptMu.RLock()
	interrupted := exec.interrupted
	exec.interruptMu.RUnlock()
	if !interrupted {
		t.Fatal("exec.interrupted must be set on recovery")
	}
	if exec.ErrorMessage != "" {
		t.Fatalf("ErrorMessage=%q, want empty (not a failure)", exec.ErrorMessage)
	}
	if exec.FinishedAt != nil {
		t.Fatal("waiting_human must not stamp FinishedAt (non-terminal)")
	}
	if got := repo.runs[exec.ID].Status; got != string(GraphExecWaitingHuman) {
		t.Fatalf("persisted status=%q, want waiting_human", got)
	}
}

// pin②：无 interrupt checkpoint（最新来源非 interrupt / 空页 / 探测失败）
// ⇒ 保持 N1 fail-closed 语义（running 流无 done 结束仍判 failed）。
func TestConvergeStreamEnd_NoInterruptCheckpointKeepsFailClosed(t *testing.T) {
	cases := map[string]*interruptProbeRuntime{
		"latest checkpoint is not interrupt": {list: GraphCheckpointList{{
			Ref:    GraphCheckpointRef{LineageID: "lin-1", CheckpointID: "cp-9"},
			Source: "loop",
			Step:   4,
		}}},
		"empty history":  {list: nil},
		"probe error":    {err: errors.New("saver down")},
		"nil runtime ok": nil,
	}
	for name, rt := range cases {
		t.Run(name, func(t *testing.T) {
			exec := runningRecoverExec("exec-h1-" + name)
			if rt != nil { // rt 为 nil 时保持 exec.runtime 为真正 nil 接口
				exec.runtime = rt
			}
			repo := &memGraphRunRepo{runs: map[string]*GraphExecution{exec.ID: exec}}
			uc := newRecoverTestUsecase(repo, nil, nil, exec.ID)

			uc.convergeStreamEnd(exec, exec.streamGen, exec.ID, exec.GraphID, false, nil)

			if exec.Status != string(GraphExecFailed) {
				t.Fatalf("status=%q, want failed (no durable interrupt evidence)", exec.Status)
			}
			if exec.ErrorMessage == "" {
				t.Fatal("fail-closed must record an error message")
			}
		})
	}
}

// pin④（V-1）：最新 checkpoint 虽为 interrupt 源但与 resume 起点 floor
// 等值 ⇒ 陈旧证据，保持 fail-closed（failed）；floor 之后新写的 interrupt
// checkpoint（ID 不同）仍须恢复 waiting_human。
func TestConvergeStreamEnd_ResumeFloorCheckpointIsStale(t *testing.T) {
	t.Run("floor-equal checkpoint keeps fail-closed", func(t *testing.T) {
		exec := convergeInterruptExec("exec-v1-stale", &interruptProbeRuntime{
			list: GraphCheckpointList{{
				Ref:    GraphCheckpointRef{LineageID: "lin-1", CheckpointID: "cp-old"},
				Source: graphCheckpointSourceInterrupt,
				Step:   3,
			}},
		})
		exec.resumeProbeFloorCkptID = "cp-old"
		repo := &memGraphRunRepo{runs: map[string]*GraphExecution{exec.ID: exec}}
		uc := newRecoverTestUsecase(repo, nil, nil, exec.ID)

		uc.convergeStreamEnd(exec, exec.streamGen, exec.ID, exec.GraphID, false, nil)

		if exec.Status != string(GraphExecFailed) {
			t.Fatalf("status=%q, want failed (stale resume-point interrupt is not new evidence)", exec.Status)
		}
	})
	t.Run("interrupt checkpoint newer than floor still recovers", func(t *testing.T) {
		exec := convergeInterruptExec("exec-v1-new", &interruptProbeRuntime{
			list: GraphCheckpointList{{
				Ref:    GraphCheckpointRef{LineageID: "lin-1", CheckpointID: "cp-new"},
				Source: graphCheckpointSourceInterrupt,
				Step:   5,
			}},
		})
		exec.resumeProbeFloorCkptID = "cp-old"
		repo := &memGraphRunRepo{runs: map[string]*GraphExecution{exec.ID: exec}}
		uc := newRecoverTestUsecase(repo, nil, nil, exec.ID)

		uc.convergeStreamEnd(exec, exec.streamGen, exec.ID, exec.GraphID, false, nil)

		if exec.Status != string(GraphExecWaitingHuman) {
			t.Fatalf("status=%q, want waiting_human (interrupt written after resume must recover)", exec.Status)
		}
	})
}

// pin③：字面量锚——biz 侧探针常量必须与 trpc graph 框架写入 checkpoint
// metadata 的 Source 值一致（executor handleInterrupt PutFull）。
func TestGraphCheckpointSourceInterruptMatchesFramework(t *testing.T) {
	if graphCheckpointSourceInterrupt != trpcgraph.CheckpointSourceInterrupt {
		t.Fatalf("source const drifted from framework: %q vs %q",
			graphCheckpointSourceInterrupt, trpcgraph.CheckpointSourceInterrupt)
	}
}
