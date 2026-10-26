package runtime

import (
	"testing"
	"time"
)

// V 桶 minor pin：B1 修复让 paused 条目永不被 Finish 清除，但此前没有任何
// 过期路径——长跑进程内存随暂停会话数单调增长。PausedStatusTTL 惰性过期后
// ResumeSession 仍有 S-3 会话状态水合回退，语义不破。

func TestRunRegistryPausedStatusExpiresAfterTTL(t *testing.T) {
	reg := NewRunRegistry()
	// 直接写入背时条目（SetStatus 恒用 time.Now，包内测试直写底层 store）。
	reg.runStatuses.store("s1", &RunStatusEntry{
		RunID:     "r1",
		Status:    RunStatusPaused,
		UpdatedAt: time.Now().Add(-PausedStatusTTL - time.Minute),
	})

	if _, ok := reg.GetStatus("s1"); ok {
		t.Fatal("paused entry older than PausedStatusTTL must be lazily expired")
	}
	// 惰性删除须真实清表，不是每次读都拦。
	if _, ok := reg.runStatuses.load("s1"); ok {
		t.Fatal("expired paused entry must be deleted, not just hidden")
	}
}

func TestRunRegistryPausedStatusFreshEntryKept(t *testing.T) {
	reg := NewRunRegistry()
	reg.SetStatus("s1", "r1", RunStatusPaused, "")

	st, ok := reg.GetStatus("s1")
	if !ok || st.Status != RunStatusPaused {
		t.Fatalf("fresh paused entry must survive: %+v ok=%v (B1 regression guard)", st, ok)
	}
}
