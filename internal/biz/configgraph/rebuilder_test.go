package configgraph

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type fakeRepo struct {
	mu           sync.Mutex
	nodeIDs      [][]string // sorted node-id snapshot per upsert
	edgeIDs      [][]string
	maxGen       int64
	belowCh      chan int64
	upsertNodesE error
	upsertEdgesE error
}

func newFakeRepo() *fakeRepo { return &fakeRepo{belowCh: make(chan int64, 4)} }

func sortedIDs[T ~string](ids []T) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	sort.Strings(out)
	return out
}

func (f *fakeRepo) UpsertNodes(_ context.Context, nodes []Node) error {
	if f.upsertNodesE != nil {
		return f.upsertNodesE
	}
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	f.mu.Lock()
	f.nodeIDs = append(f.nodeIDs, sortedIDs(ids))
	f.mu.Unlock()
	return nil
}

func (f *fakeRepo) UpsertEdges(_ context.Context, edges []StoredEdge) error {
	if f.upsertEdgesE != nil {
		return f.upsertEdgesE
	}
	ids := make([]string, 0, len(edges))
	for _, e := range edges {
		ids = append(ids, e.ID)
	}
	f.mu.Lock()
	f.edgeIDs = append(f.edgeIDs, sortedIDs(ids))
	f.mu.Unlock()
	return nil
}

func (f *fakeRepo) MaxGeneration(context.Context) (int64, error) { return f.maxGen, nil }

func (f *fakeRepo) DeleteGenerationBelow(_ context.Context, belowGen int64) (int64, error) {
	f.belowCh <- belowGen
	return 3, nil
}

func (f *fakeRepo) DeleteOutEdges(context.Context, string, int64) error   { return nil }
func (f *fakeRepo) ListNodes(context.Context, NodeFilter) ([]Node, error) { return nil, nil }
func (f *fakeRepo) Counts(context.Context, int64) (Counts, error)         { return Counts{}, nil }

// P1 查询方法 stub（querier 单测用 queryFakeRepo 覆盖行为）。
func (f *fakeRepo) FindNode(context.Context, int64, string, string) (Node, error) {
	return Node{}, errFakeNotFound
}
func (f *fakeRepo) WalkGraph(context.Context, int64, string, bool, int) ([]WalkRow, error) {
	return nil, nil
}
func (f *fakeRepo) ListNodeEdges(context.Context, int64, string) ([]StoredEdge, []StoredEdge, []StoredEdge, error) {
	return nil, nil, nil, nil
}
func (f *fakeRepo) ListBrokenEdgesTargeting(context.Context, int64, []string) ([]StoredEdge, error) {
	return nil, nil
}
func (f *fakeRepo) CountActiveSessions(context.Context, []string, []string) (int64, error) {
	return 0, nil
}
func (f *fakeRepo) ListAllEdges(context.Context, int64) ([]StoredEdge, error) { return nil, nil }
func (f *fakeRepo) ListAllNodes(context.Context, int64) ([]Node, error)       { return nil, nil }

type fakeFlowLog struct {
	mu     sync.Mutex
	starts []string
	dones  []string
	errs   []string
}

func (f *fakeFlowLog) LogFlowStart(_ context.Context, _, stepID, _ string, _ ...monitor.LogPair) {
	f.mu.Lock()
	f.starts = append(f.starts, stepID)
	f.mu.Unlock()
}
func (f *fakeFlowLog) LogFlowDone(_ context.Context, _, stepID, _ string, _ ...monitor.LogPair) {
	f.mu.Lock()
	f.dones = append(f.dones, stepID)
	f.mu.Unlock()
}
func (f *fakeFlowLog) LogFlowError(_ context.Context, _, stepID, _ string, _ ...monitor.LogPair) {
	f.mu.Lock()
	f.errs = append(f.errs, stepID)
	f.mu.Unlock()
}

// ── tests ────────────────────────────────────────────────────────────────────

func waitBelow(t *testing.T, repo *fakeRepo) int64 {
	t.Helper()
	select {
	case g := <-repo.belowCh:
		return g
	case <-time.After(5 * time.Second):
		t.Fatal("cleanupBelow not invoked within 5s")
		return 0
	}
}

func TestRebuilder_FullRebuildAndIdempotency(t *testing.T) {
	src, prov := fullFixture()
	repo := newFakeRepo()
	flow := &fakeFlowLog{}
	rb := NewRebuilder(src, repo, prov, flow, nil)
	if rb == nil || rb.Ready() || rb.Current() != 0 {
		t.Fatalf("fresh rebuilder must be unready at gen 0: %+v", rb)
	}

	res1, err := rb.Rebuild(context.Background())
	if err != nil {
		t.Fatalf("rebuild 1: %v", err)
	}
	if res1.Generation != 1 || res1.Nodes == 0 || res1.Edges == 0 {
		t.Fatalf("res1: %+v", res1)
	}
	if !rb.Ready() || rb.Current() != 1 {
		t.Fatalf("after rebuild 1: ready=%v current=%d", rb.Ready(), rb.Current())
	}
	// gen=1 → belowGen=0 → cleanupBelow 提前 return，不应有任何清理信号。
	select {
	case g := <-repo.belowCh:
		t.Fatalf("rebuild 1 must skip cleanup (below=0), got %d", g)
	case <-time.After(300 * time.Millisecond):
	}

	res2, err := rb.Rebuild(context.Background())
	if err != nil {
		t.Fatalf("rebuild 2: %v", err)
	}
	if res2.Generation != 2 || res2.Nodes != res1.Nodes || res2.Edges != res1.Edges || res2.Broken != res1.Broken {
		t.Fatalf("res2 vs res1 mismatch: %+v vs %+v", res2, res1)
	}
	// 幂等：两代写入的确定性 ID 集合字节一致。
	if !equalStringSlices(repo.nodeIDs[0], repo.nodeIDs[1]) || !equalStringSlices(repo.edgeIDs[0], repo.edgeIDs[1]) {
		t.Fatal("rebuild not byte-stable: node/edge id sets differ across generations")
	}
	if g := waitBelow(t, repo); g != 1 {
		t.Fatalf("rebuild 2 cleanup belowGen = %d, want 1", g)
	}

	// flowlog：start/done 各两次，step 恒定。
	if len(flow.starts) != 2 || len(flow.dones) != 2 || len(flow.errs) != 0 {
		t.Fatalf("flowlog calls: starts=%d dones=%d errs=%d", len(flow.starts), len(flow.dones), len(flow.errs))
	}
	for _, s := range append(append([]string{}, flow.starts...), flow.dones...) {
		if s != flowStepRebuild {
			t.Fatalf("flowlog step = %q, want %q", s, flowStepRebuild)
		}
	}
}

func TestRebuilder_InitSeedsGeneration(t *testing.T) {
	src, prov := fullFixture()
	repo := newFakeRepo()
	repo.maxGen = 7
	rb := NewRebuilder(src, repo, prov, nil, nil)
	if err := rb.Init(context.Background()); err != nil {
		t.Fatalf("init: %v", err)
	}
	if rb.Current() != 7 || !rb.Ready() {
		t.Fatalf("after init: current=%d ready=%v", rb.Current(), rb.Ready())
	}
	res, err := rb.Rebuild(context.Background())
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if res.Generation != 8 {
		t.Fatalf("generation = %d, want 8", res.Generation)
	}
}

func TestRebuilder_UpsertFailureKeepsGeneration(t *testing.T) {
	src, prov := fullFixture()
	repo := newFakeRepo()
	repo.upsertEdgesE = errors.New("pg down")
	flow := &fakeFlowLog{}
	rb := NewRebuilder(src, repo, prov, flow, nil)

	if _, err := rb.Rebuild(context.Background()); err == nil {
		t.Fatal("want error")
	}
	if rb.Ready() || rb.Current() != 0 {
		t.Fatalf("failed rebuild must not switch generation: current=%d ready=%v", rb.Current(), rb.Ready())
	}
	if len(flow.errs) != 1 {
		t.Fatalf("flowlog errors = %d, want 1", len(flow.errs))
	}
}

func TestRebuilder_RunningAndLastRebuild(t *testing.T) {
	src, prov := fullFixture()
	repo := newFakeRepo()
	rb := NewRebuilder(src, repo, prov, nil, nil)

	if rb.Running() {
		t.Fatal("fresh rebuilder must not be running")
	}
	if _, _, ok := rb.LastRebuild(); ok {
		t.Fatal("fresh rebuilder must have no last rebuild")
	}

	res, err := rb.Rebuild(context.Background())
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if rb.Running() {
		t.Fatal("running must clear after synchronous rebuild")
	}
	got, at, ok := rb.LastRebuild()
	if !ok {
		t.Fatal("last rebuild snapshot missing after success")
	}
	if got.Generation != res.Generation || got.Nodes != res.Nodes || got.Edges != res.Edges {
		t.Fatalf("snapshot = %+v, want %+v", got, res)
	}
	if at.IsZero() || time.Since(at) > time.Minute {
		t.Fatalf("snapshot time = %v", at)
	}
}

func TestRebuilder_RebuildAsyncDedup(t *testing.T) {
	src, prov := fullFixture()
	repo := newFakeRepo()
	rb := NewRebuilder(src, repo, prov, nil, nil)

	gen, started := rb.RebuildAsync()
	if !started || gen != 1 {
		t.Fatalf("first async: gen=%d started=%v, want (1,true)", gen, started)
	}
	// 异步重建完成前重复触发必须被拒（在途去重）。异步重建极快时可能已结束，
	// 此时 started=true 也合法——但 gen 必须单调到 2。
	gen2, started2 := rb.RebuildAsync()
	if !started2 && gen2 != gen {
		t.Fatalf("in-flight dedup must report same generation: gen2=%d gen=%d", gen2, gen)
	}

	// 等所有在途重建落定。
	deadline := time.Now().Add(5 * time.Second)
	for rb.Running() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if rb.Running() {
		t.Fatal("async rebuild did not settle within 5s")
	}
	if !rb.Ready() || rb.Current() < 1 {
		t.Fatalf("after async: current=%d ready=%v", rb.Current(), rb.Ready())
	}
	if _, _, ok := rb.LastRebuild(); !ok {
		t.Fatal("async rebuild must record last snapshot")
	}
}

func TestRebuilder_AsyncFailureKeepsGeneration(t *testing.T) {
	src, prov := fullFixture()
	repo := newFakeRepo()
	repo.upsertNodesE = errors.New("pg down")
	rb := NewRebuilder(src, repo, prov, nil, nil)

	if _, started := rb.RebuildAsync(); !started {
		t.Fatal("first async must start")
	}
	deadline := time.Now().Add(5 * time.Second)
	for rb.Running() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if rb.Running() {
		t.Fatal("failed async rebuild must release running flag")
	}
	if rb.Ready() || rb.Current() != 0 {
		t.Fatalf("failed async must not switch generation: current=%d ready=%v", rb.Current(), rb.Ready())
	}
	if _, _, ok := rb.LastRebuild(); ok {
		t.Fatal("failed async must not record snapshot")
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
