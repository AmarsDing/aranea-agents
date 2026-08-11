package voice

import (
	"fmt"
	"sync"
	"testing"
)

func TestDelegationRegistry_RegisterBindComplete(t *testing.T) {
	r := NewDelegationRegistry(nil)
	regID := r.Register("vs-1", "ss-1", "任务A")
	if regID == 0 {
		t.Fatal("Register returned 0")
	}
	// OwnerOf：pending 条目已可判定归属（三路分流第二路）
	if vs, ok := r.OwnerOf("ss-1"); !ok || vs != "vs-1" {
		t.Fatalf("OwnerOf=(%q,%v)", vs, ok)
	}
	// 内容精确匹配绑定（FIFO）
	vs, ok := r.BindTask("ss-1", "任务A", "task-1")
	if !ok || vs != "vs-1" {
		t.Fatalf("BindTask=(%q,%v)", vs, ok)
	}
	// 重复绑定不匹配（已 bound）
	if _, ok := r.BindTask("ss-1", "任务A", "task-2"); ok {
		t.Fatal("re-bind should fail")
	}
	// 终态取出并移除（owner 限定：第一参 voice session）
	e, ok := r.CompleteTask("vs-1", "ss-1", "task-1")
	if !ok || e.VoiceSessionID != "vs-1" || e.Content != "任务A" {
		t.Fatalf("CompleteTask=(%+v,%v)", e, ok)
	}
	if _, ok := r.OwnerOf("ss-1"); ok {
		t.Fatal("entry should be removed after CompleteTask")
	}
}

// owner 限定消费（M74 V9 R3）：总线全量广播下，非所有者会话调用
// CompleteTask 不得消费条目（防截胡导致所有者播报丢失）。
func TestDelegationRegistry_CompleteTaskOwnerScoped(t *testing.T) {
	r := NewDelegationRegistry(nil)
	r.Register("vs-1", "ss-1", "任务")
	if _, ok := r.BindTask("ss-1", "任务", "task-1"); !ok {
		t.Fatal("bind failed")
	}
	// 非所有者（其他 voice session / 空串）不得消费
	if _, ok := r.CompleteTask("vs-2", "ss-1", "task-1"); ok {
		t.Fatal("non-owner must not consume the entry")
	}
	if _, ok := r.CompleteTask("", "ss-1", "task-1"); ok {
		t.Fatal("empty voice session must not consume the entry")
	}
	// 条目仍在，所有者可消费
	e, ok := r.CompleteTask("vs-1", "ss-1", "task-1")
	if !ok || e.VoiceSessionID != "vs-1" {
		t.Fatalf("owner CompleteTask=(%+v,%v)", e, ok)
	}
}

func TestDelegationRegistry_BindFIFO(t *testing.T) {
	r := NewDelegationRegistry(nil)
	// 同内容重复委派：TaskCreated 顺序与提交顺序一致（FIFO 绑定）
	r.Register("vs-1", "ss-1", "相同任务")
	r.Register("vs-2", "ss-1", "相同任务")
	vs, ok := r.BindTask("ss-1", "相同任务", "task-first")
	if !ok || vs != "vs-1" {
		t.Fatalf("first bind should hit earliest pending, got (%q,%v)", vs, ok)
	}
	vs, ok = r.BindTask("ss-1", "相同任务", "task-second")
	if !ok || vs != "vs-2" {
		t.Fatalf("second bind should hit next pending, got (%q,%v)", vs, ok)
	}
}

func TestDelegationRegistry_BindRejectsForeignContent(t *testing.T) {
	r := NewDelegationRegistry(nil)
	r.Register("vs-1", "ss-1", "我的任务")
	// 外来 turn（同 spirit 会话、不同内容）不得错绑
	if _, ok := r.BindTask("ss-1", "别人的输入", "task-x"); ok {
		t.Fatal("foreign content must not bind")
	}
	// 其他 spirit 会话不得错绑
	if _, ok := r.BindTask("ss-other", "我的任务", "task-y"); ok {
		t.Fatal("foreign spirit session must not bind")
	}
}

func TestDelegationRegistry_MarkSubmitFailed_NotifiesWatcher(t *testing.T) {
	r := NewDelegationRegistry(nil)
	var mu sync.Mutex
	var got DelegationNotice
	r.SetWatcher("vs-1", func(n DelegationNotice) {
		mu.Lock()
		got = n
		mu.Unlock()
	})
	regID := r.Register("vs-1", "ss-1", "任务B")
	r.MarkSubmitFailed(regID, "提交失败口播")
	mu.Lock()
	defer mu.Unlock()
	if got.Kind != NoticeDelegationSubmitFailed || got.Message != "提交失败口播" {
		t.Fatalf("notice=%+v", got)
	}
	// 条目已移除
	if _, ok := r.OwnerOf("ss-1"); ok {
		t.Fatal("entry should be removed after MarkSubmitFailed")
	}
}

func TestDelegationRegistry_ClearVoiceSession(t *testing.T) {
	r := NewDelegationRegistry(nil)
	r.Register("vs-1", "ss-1", "任务C")
	r.Register("vs-1", "ss-2", "任务D")
	r.Register("vs-2", "ss-3", "任务E")
	r.SetWatcher("vs-1", func(DelegationNotice) {})
	r.ClearVoiceSession("vs-1")
	if _, ok := r.OwnerOf("ss-1"); ok {
		t.Fatal("ss-1 entry should be cleared")
	}
	if _, ok := r.OwnerOf("ss-2"); ok {
		t.Fatal("ss-2 entry should be cleared")
	}
	if vs, ok := r.OwnerOf("ss-3"); !ok || vs != "vs-2" {
		t.Fatal("vs-2 entry must survive")
	}
}

func TestDelegationRegistry_CapEvictsOldest(t *testing.T) {
	r := NewDelegationRegistry(nil)
	for i := 0; i < delegationMaxEntries; i++ {
		r.Register("vs-1", fmt.Sprintf("ss-%d", i), "任务")
	}
	// 超限淘汰最旧
	r.Register("vs-1", "ss-new", "任务")
	if _, ok := r.OwnerOf("ss-0"); ok {
		t.Fatal("oldest entry should be evicted")
	}
	if _, ok := r.OwnerOf("ss-new"); !ok {
		t.Fatal("new entry should be present")
	}
}

func TestDelegationRegistry_NilSafe(t *testing.T) {
	var r *DelegationRegistry
	if id := r.Register("a", "b", "c"); id != 0 {
		t.Fatal("nil Register should return 0")
	}
	if _, ok := r.BindTask("a", "b", "c"); ok {
		t.Fatal("nil BindTask should return false")
	}
	if _, ok := r.CompleteTask("a", "b", "c"); ok {
		t.Fatal("nil CompleteTask should return false")
	}
	if _, ok := r.OwnerOf("a"); ok {
		t.Fatal("nil OwnerOf should return false")
	}
	r.MarkSubmitFailed(1, "x") // no panic
	r.SetWatcher("a", nil)     // no panic
	r.ClearVoiceSession("a")   // no panic
}
