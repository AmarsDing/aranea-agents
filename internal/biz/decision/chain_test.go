package decision

import (
	"context"
	"testing"
)

// fakeChainQueryRepo 同时实现 QueryRepo + ChainRepo 的测试替身。
type fakeChainQueryRepo struct {
	byKey      map[string]*Record
	upstream   []Record
	downstream []Record
	planner    *Record
	lastRef    SourceRef
}

func (f *fakeChainQueryRepo) ListRecords(context.Context, ListFilter) ([]Record, int64, error) {
	return nil, 0, nil
}

func (f *fakeChainQueryRepo) GetByKey(_ context.Context, key string) (*Record, error) {
	return f.byKey[key], nil
}

func (f *fakeChainQueryRepo) ListUpstream(context.Context, int64, int) ([]Record, error) {
	return f.upstream, nil
}

func (f *fakeChainQueryRepo) ListDownstream(context.Context, int64, int) ([]Record, error) {
	return f.downstream, nil
}

func (f *fakeChainQueryRepo) FindVirtualParentPlanner(_ context.Context, ref SourceRef, _ string, _ int64) (*Record, error) {
	f.lastRef = ref
	return f.planner, nil
}

func chainTestUsecase(repo *fakeChainQueryRepo) *QueryUsecase {
	return NewQueryUsecase(repo)
}

// TestGetChain_RealParentChain pins the happy path: root 有真实父链时
// upstream 走 CTE 结果、downstream 走子链，方向 both 默认。
func TestGetChain_RealParentChain(t *testing.T) {
	parent := int64(10)
	root := &Record{ID: 11, DecisionKey: "dk-root", ParentDecisionID: &parent}
	repo := &fakeChainQueryRepo{
		byKey:      map[string]*Record{"dk-root": root},
		upstream:   []Record{{ID: 10, DecisionKey: "dk-parent"}, {ID: 9, DecisionKey: "dk-grand"}},
		downstream: []Record{{ID: 12, DecisionKey: "dk-child"}},
	}
	chain, err := chainTestUsecase(repo).GetChain(context.Background(), "dk-root", "", 0)
	if err != nil || chain == nil {
		t.Fatalf("GetChain = (%v,%v)", chain, err)
	}
	if len(chain.Upstream) != 2 || chain.Upstream[0].DecisionKey != "dk-parent" {
		t.Fatalf("upstream = %+v", chain.Upstream)
	}
	if chain.Upstream[0].VirtualParent {
		t.Error("real parent must not be marked virtual")
	}
	if len(chain.Downstream) != 1 || chain.Downstream[0].DecisionKey != "dk-child" {
		t.Fatalf("downstream = %+v", chain.Downstream)
	}
}

// TestGetChain_VirtualParentFallback pins 设计 §5 兜底：root 无父且带
// run_id 时，同 run 最近前置 planner 决策作为虚拟父返回（仅单节点）。
func TestGetChain_VirtualParentFallback(t *testing.T) {
	root := &Record{ID: 20, DecisionKey: "dk-gate", SourceRef: SourceRef{RunID: "run-1"}, CreatedAt: "2026-08-26T03:00:00Z"}
	repo := &fakeChainQueryRepo{
		byKey:   map[string]*Record{"dk-gate": root},
		planner: &Record{ID: 5, DecisionKey: "dk-planner", Category: CategoryPlannerOrchestration},
	}
	chain, err := chainTestUsecase(repo).GetChain(context.Background(), "dk-gate", "up", 20)
	if err != nil || chain == nil {
		t.Fatalf("GetChain = (%v,%v)", chain, err)
	}
	if len(chain.Upstream) != 1 || !chain.Upstream[0].VirtualParent {
		t.Fatalf("virtual parent = %+v", chain.Upstream)
	}
	if chain.Upstream[0].DecisionKey != "dk-planner" {
		t.Fatalf("virtual parent key = %q", chain.Upstream[0].DecisionKey)
	}
}

// TestGetChain_NoParentNoRun：无父且无 run_id → upstream 为空。
func TestGetChain_NoParentNoRun(t *testing.T) {
	root := &Record{ID: 30, DecisionKey: "dk-lonely"}
	repo := &fakeChainQueryRepo{byKey: map[string]*Record{"dk-lonely": root}}
	chain, err := chainTestUsecase(repo).GetChain(context.Background(), "dk-lonely", "both", 20)
	if err != nil || chain == nil {
		t.Fatalf("GetChain = (%v,%v)", chain, err)
	}
	if len(chain.Upstream) != 0 {
		t.Fatalf("upstream = %+v, want empty", chain.Upstream)
	}
}

// TestGetChain_CycleDedup pins the 环检测契约（设计 §5 路径集合去重）：
// 脏数据自环（root 出现在下游/祖先中、重复 ID）必须被剔除。
func TestGetChain_CycleDedup(t *testing.T) {
	parent := int64(40)
	root := &Record{ID: 41, DecisionKey: "dk-cyc", ParentDecisionID: &parent}
	repo := &fakeChainQueryRepo{
		byKey: map[string]*Record{"dk-cyc": root},
		// 脏数据：祖先链里出现 root 自身 + 重复 ID。
		upstream:   []Record{{ID: 40, DecisionKey: "dk-p"}, {ID: 41, DecisionKey: "dk-cyc"}, {ID: 40, DecisionKey: "dk-p"}},
		downstream: []Record{{ID: 42, DecisionKey: "dk-c"}, {ID: 42, DecisionKey: "dk-c"}, {ID: 41, DecisionKey: "dk-cyc"}},
	}
	chain, err := chainTestUsecase(repo).GetChain(context.Background(), "dk-cyc", "both", 20)
	if err != nil || chain == nil {
		t.Fatalf("GetChain = (%v,%v)", chain, err)
	}
	if len(chain.Upstream) != 1 || chain.Upstream[0].ID != 40 {
		t.Fatalf("upstream dedup = %+v", chain.Upstream)
	}
	if len(chain.Downstream) != 1 || chain.Downstream[0].ID != 42 {
		t.Fatalf("downstream dedup = %+v", chain.Downstream)
	}
}

// TestGetChain_Direction 覆盖方向枚举：up 不查下游、down 不查上游。
func TestGetChain_Direction(t *testing.T) {
	parent := int64(50)
	root := &Record{ID: 51, DecisionKey: "dk-dir", ParentDecisionID: &parent}
	repo := &fakeChainQueryRepo{
		byKey:      map[string]*Record{"dk-dir": root},
		upstream:   []Record{{ID: 50, DecisionKey: "dk-p"}},
		downstream: []Record{{ID: 52, DecisionKey: "dk-c"}},
	}
	upOnly, err := chainTestUsecase(repo).GetChain(context.Background(), "dk-dir", "up", 20)
	if err != nil || len(upOnly.Upstream) != 1 || len(upOnly.Downstream) != 0 {
		t.Fatalf("up: %+v err=%v", upOnly, err)
	}
	downOnly, err := chainTestUsecase(repo).GetChain(context.Background(), "dk-dir", "down", 20)
	if err != nil || len(downOnly.Upstream) != 0 || len(downOnly.Downstream) != 1 {
		t.Fatalf("down: %+v err=%v", downOnly, err)
	}
}

// TestGetChain_NotFound / nil repo：锚点未命中或无库 → nil, nil。
func TestGetChain_NotFound(t *testing.T) {
	repo := &fakeChainQueryRepo{byKey: map[string]*Record{}}
	chain, err := chainTestUsecase(repo).GetChain(context.Background(), "dk-x", "both", 20)
	if err != nil || chain != nil {
		t.Fatalf("notfound = (%v,%v)", chain, err)
	}
	chain, err = NewQueryUsecase(nil).GetChain(context.Background(), "dk-x", "both", 20)
	if err != nil || chain != nil {
		t.Fatalf("nil repo = (%v,%v)", chain, err)
	}
}

// TestNormalizeChainParams pins 参数收敛：空 direction → both；max_depth
// 越界 → 20。
func TestNormalizeChainParams(t *testing.T) {
	d, depth := normalizeChainParams("", 0)
	if d != ChainBoth || depth != 20 {
		t.Errorf("default = (%q,%d)", d, depth)
	}
	d, depth = normalizeChainParams("UP", 99)
	if d != ChainUp || depth != 20 {
		t.Errorf("clamp = (%q,%d)", d, depth)
	}
	d, depth = normalizeChainParams("down", 5)
	if d != ChainDown || depth != 5 {
		t.Errorf("valid = (%q,%d)", d, depth)
	}
}
