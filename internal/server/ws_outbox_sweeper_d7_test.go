package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// --- D7-2：ARANEA_OUTBOX_SWEEP_CLAIM=on 认领化清扫钉板（deferred 战役） ---

// TestResolveOutboxSweepClaimMode 钉死 flag 解析矩阵：off 缺省/非法值回落，
// 大小写/空白容忍，布尔习惯值回落 off（枚举不接受布尔真值）。
func TestResolveOutboxSweepClaimMode(t *testing.T) {
	t.Parallel()
	if got := ResolveOutboxSweepClaimMode(nil); got != OutboxSweepClaimOff {
		t.Fatalf("nil getenv = %q, want off", got)
	}
	for _, c := range []struct{ in, want string }{
		{"", OutboxSweepClaimOff},
		{"off", OutboxSweepClaimOff},
		{"on", OutboxSweepClaimOn},
		{"ON", OutboxSweepClaimOn},
		{" on ", OutboxSweepClaimOn},
		{"true", OutboxSweepClaimOff},
		{"1", OutboxSweepClaimOff},
		{"pg", OutboxSweepClaimOff},
	} {
		getenv := func(string) string { return c.in }
		if got := ResolveOutboxSweepClaimMode(getenv); got != c.want {
			t.Errorf("ResolveOutboxSweepClaimMode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// d7ClaimOutbox 在 fakePendingOutbox 上叠加认领端口（biz.EventOutboxClaimer）：
// 认领记录调用参数并回放固定批次；claimErr 注入故障。
type d7ClaimOutbox struct {
	fakePendingOutbox
	claimCalls  int
	claimWorker string
	claimCutoff time.Time
	claimLease  time.Time
	claimLimit  int
	claimErr    error
	claimRows   []biz.EventDeliveryOutboxRow
}

func (f *d7ClaimOutbox) ClaimPendingOutboxDeliveries(_ context.Context, workerID string, cutoff, claimLeaseDeadline time.Time, limit int) ([]biz.EventDeliveryOutboxRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claimCalls++
	f.claimWorker = workerID
	f.claimCutoff = cutoff
	f.claimLease = claimLeaseDeadline
	f.claimLimit = limit
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	return append([]biz.EventDeliveryOutboxRow(nil), f.claimRows...), nil
}

// TestSweepOnceClaimMode_ClaimsBroadcastsMarks：认领档批次来源 = 认领端口
// （ListPending 零调用），认领参数贯通（workerID/limit/租约死线≈now-lease），
// 广播与 MarkPublishedBatch 语义与旧档一致。
func TestSweepOnceClaimMode_ClaimsBroadcastsMarks(t *testing.T) {
	t.Parallel()

	repo := &d7ClaimOutbox{claimRows: []biz.EventDeliveryOutboxRow{
		{ID: "r1", SessionID: "sess-1", Seq: 7, EventID: "evt-7", Payload: []byte(`{"event_id":"evt-7"}`)},
		{ID: "r2", SessionID: "sess-2", Seq: 3, EventID: "evt-3", Payload: []byte(`{"event_id":"evt-3"}`)},
	}}
	var broadcasts []string
	s := newEventOutboxSweeper(repo, func(sessionID string, msg []byte) {
		broadcasts = append(broadcasts, sessionID)
	}, loggateway.NewNoop())
	s.EnableClaiming(repo, "worker-a", 30*time.Second)

	before := time.Now().UTC()
	s.sweepOnce(context.Background())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.claimCalls != 1 {
		t.Fatalf("claim calls = %d, want 1", repo.claimCalls)
	}
	if repo.claimWorker != "worker-a" {
		t.Fatalf("claim worker = %q, want worker-a", repo.claimWorker)
	}
	if repo.claimLimit != defaultOutboxSweepLimit {
		t.Fatalf("claim limit = %d, want %d", repo.claimLimit, defaultOutboxSweepLimit)
	}
	// 租约死线 = now-lease（前后 5s 容差吸收测试时钟漂移）。
	wantLease := before.Add(-30 * time.Second)
	if repo.claimLease.Before(wantLease.Add(-5*time.Second)) || repo.claimLease.After(time.Now().UTC().Add(-25*time.Second)) {
		t.Fatalf("claim lease deadline = %v, want ≈ %v", repo.claimLease, wantLease)
	}
	if len(repo.pendingSeen) != 0 {
		t.Fatalf("认领档不得触碰 ListPending，calls = %d", len(repo.pendingSeen))
	}
	if len(broadcasts) != 2 {
		t.Fatalf("broadcasts = %v, want 2 场", broadcasts)
	}
	if len(repo.markCalls) != 2 || repo.markCalls[0] != "r1" || repo.markCalls[1] != "r2" {
		t.Fatalf("markCalls = %v, want [r1 r2]", repo.markCalls)
	}
}

// TestSweepOnceClaimMode_ClaimErrorSkipsTick：认领失败本拍全跳（无广播无
// 打标无 panic），下一拍重试——不回落 ListPending（双源混用会破坏认领
// 排他语义）。
func TestSweepOnceClaimMode_ClaimErrorSkipsTick(t *testing.T) {
	t.Parallel()

	repo := &d7ClaimOutbox{claimErr: errors.New("claim tx failed")}
	broadcastCount := 0
	s := newEventOutboxSweeper(repo, func(string, []byte) { broadcastCount++ }, loggateway.NewNoop())
	s.EnableClaiming(repo, "worker-a", 0)

	s.sweepOnce(context.Background())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.claimCalls != 1 {
		t.Fatalf("claim calls = %d, want 1", repo.claimCalls)
	}
	if broadcastCount != 0 {
		t.Fatalf("认领失败不得广播，count = %d", broadcastCount)
	}
	if len(repo.markCalls) != 0 {
		t.Fatalf("认领失败不得打标，markCalls = %v", repo.markCalls)
	}
	if len(repo.pendingSeen) != 0 {
		t.Fatalf("认领失败不得回落 ListPending，calls = %d", len(repo.pendingSeen))
	}
}

// TestSweepOnceOffMode_ListPendingByteLevelUnchanged：未开启认领（off 档
// 字节级旧行为）——批次来源仍是 ListPending，认领端口零调用。
func TestSweepOnceOffMode_ListPendingByteLevelUnchanged(t *testing.T) {
	t.Parallel()

	repo := &d7ClaimOutbox{}
	repo.pending = []biz.EventDeliveryOutboxRow{
		{ID: "r9", SessionID: "sess-9", Seq: 1, EventID: "evt-9", Payload: []byte(`{"event_id":"evt-9"}`)},
	}
	broadcastCount := 0
	s := newEventOutboxSweeper(repo, func(string, []byte) { broadcastCount++ }, loggateway.NewNoop())

	s.sweepOnce(context.Background())

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.claimCalls != 0 {
		t.Fatalf("off 档不得触碰认领端口，calls = %d", repo.claimCalls)
	}
	if len(repo.pendingSeen) != 1 {
		t.Fatalf("off 档 ListPending calls = %d, want 1", len(repo.pendingSeen))
	}
	if broadcastCount != 1 {
		t.Fatalf("broadcasts = %d, want 1", broadcastCount)
	}
}

// TestEnableClaiming_Defaults：workerID 缺省 = 主机名:PID 非空形态；
// lease<=0 保留缺省租约（防误配零租约=行永不钉住=多副本退化为全扫）。
func TestEnableClaiming_Defaults(t *testing.T) {
	t.Parallel()

	repo := &d7ClaimOutbox{}
	s := newEventOutboxSweeper(repo, func(string, []byte) {}, loggateway.NewNoop())
	s.EnableClaiming(repo, "", 0)
	if s.workerID == "" || !strings.Contains(s.workerID, ":") {
		t.Fatalf("缺省 workerID = %q, want host:pid 形态", s.workerID)
	}
	if s.claimLease != defaultOutboxSweepClaimLease {
		t.Fatalf("缺省租约 = %v, want %v", s.claimLease, defaultOutboxSweepClaimLease)
	}
	s.EnableClaiming(repo, "w-explicit", 5*time.Second)
	if s.workerID != "w-explicit" || s.claimLease != 5*time.Second {
		t.Fatalf("显式覆盖未生效：worker=%q lease=%v", s.workerID, s.claimLease)
	}
}
