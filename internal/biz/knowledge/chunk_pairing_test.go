package knowledge

import (
	"strings"
	"testing"
)

// chunk_pairing_test.go G-32 T2 夹具矩阵（ADR-KN-20261022-I 测试矩阵 ②；
// 对抗评审 CON-16/17/18 + DIS-4/6 后扩阵）：PairChunksForDiff 纯函数确定性钉——
// 四策略形态（flat 定窗/token、递归 markdown=flat rows、parent_child 父子）×
// 文首/中部/尾部插入 × 重复内容 × 角色误配 × 父子约束 × 对称父死子随 ×
// Kept 序承诺。夹具以"分块结果数组"模拟策略产出，不依赖真实分块器。
// ParentIdx 一律 chunker ParentIndex 语义（父序号 0..P-1，非切片索引）。

func oldFlat(contents ...string) []PairingOldRow {
	rows := make([]PairingOldRow, len(contents))
	for i, c := range contents {
		rows[i] = PairingOldRow{ID: rowID(i), Content: c, ChunkIndex: i, Role: "child"}
	}
	return rows
}

func newFlat(contents ...string) []PairingNewChunk {
	ch := make([]PairingNewChunk, len(contents))
	for i, c := range contents {
		ch[i] = PairingNewChunk{Content: c, Role: "child", ParentIdx: -1}
	}
	return ch
}

func rowID(i int) string {
	return "d1-v1-ch-" + string(rune('0'+i))
}

func assertPlan(t *testing.T, plan ChunkPairPlan, keptNewN, addedN, deletedN int) {
	t.Helper()
	if len(plan.Kept) != keptNewN {
		t.Fatalf("kept mismatch: got %d want %d (%+v)", len(plan.Kept), keptNewN, plan.Kept)
	}
	if len(plan.Added) != addedN {
		t.Fatalf("added mismatch: got %d want %d (%v)", len(plan.Added), addedN, plan.Added)
	}
	if len(plan.Deleted) != deletedN {
		t.Fatalf("deleted mismatch: got %d want %d (%v)", len(plan.Deleted), deletedN, plan.Deleted)
	}
}

// assertKeptNewIdxAsc Kept 序承诺钉（头注§）：NewIdx 必须严格升序——消费方
// 按位索引 newChunks 的便利恒成立（两轮 pass 的中间序是父段+子段，此前为暗面）。
func assertKeptNewIdxAsc(t *testing.T, plan ChunkPairPlan) {
	t.Helper()
	for i := 1; i < len(plan.Kept); i++ {
		if plan.Kept[i].NewIdx <= plan.Kept[i-1].NewIdx {
			t.Fatalf("kept order violated (must be NewIdx asc): %+v", plan.Kept)
		}
	}
}

// ① 全同 → 全部配对、零增删（fast-path/hash 未变之外的兜底场景）。
func TestPairChunks_AllIdentical(t *testing.T) {
	old := oldFlat("A", "B", "C")
	newC := newFlat("A", "B", "C")
	plan := PairChunksForDiff(old, newC)
	assertPlan(t, plan, 3, 0, 0)
	assertKeptNewIdxAsc(t, plan)
	for _, p := range plan.Kept {
		if p.OldID != rowID(p.NewIdx) {
			t.Fatalf("identity pairing order broken: new[%d] -> %s", p.NewIdx, p.OldID)
		}
	}
}

// ② 尾部插入（四策略共性形态）→ 前缀全保留、仅尾块 added。
func TestPairChunks_AppendTail(t *testing.T) {
	old := oldFlat("A", "B", "C")
	newC := newFlat("A", "B", "C", "D")
	plan := PairChunksForDiff(old, newC)
	assertPlan(t, plan, 3, 1, 0)
	assertKeptNewIdxAsc(t, plan)
	if plan.Added[0] != 3 {
		t.Fatalf("added idx mismatch: got %d want 3", plan.Added[0])
	}
}

// ②b 文首插入显式钉（diff-commit 最高频编辑形态，ADR 矩阵②四向之一）：
// 新块头插 → 全旧保留、仅首块 added——内容键不受位次影响与前缀对齐无关。
func TestPairChunks_PrependHead(t *testing.T) {
	old := oldFlat("A", "B", "C")
	newC := newFlat("H", "A", "B", "C")
	plan := PairChunksForDiff(old, newC)
	assertPlan(t, plan, 3, 1, 0)
	assertKeptNewIdxAsc(t, plan)
	if plan.Added[0] != 0 {
		t.Fatalf("head insert added idx mismatch: got %v want [0]", plan.Added)
	}
	keptIDs := map[int]string{}
	for _, p := range plan.Kept {
		keptIDs[p.NewIdx] = p.OldID
	}
	if keptIDs[1] != rowID(0) || keptIDs[2] != rowID(1) || keptIDs[3] != rowID(2) {
		t.Fatalf("suffix rows not kept by content: %v", keptIDs)
	}
}

// ③ 中部编辑 → 该块 added+deleted，前后块保留（内容键不受位次影响）。
func TestPairChunks_MiddleEdit(t *testing.T) {
	old := oldFlat("A", "B", "C")
	newC := newFlat("A", "B2", "C")
	plan := PairChunksForDiff(old, newC)
	assertPlan(t, plan, 2, 1, 1)
	assertKeptNewIdxAsc(t, plan)
	if plan.Deleted[0] != rowID(1) {
		t.Fatalf("deleted id mismatch: got %s want %s", plan.Deleted[0], rowID(1))
	}
}

// ④ 重复内容块贪心确定性：同文两块，新集第一块必须按 chunk_index 升序
// 配到旧集第一块（可复现性钉——乱序配对付审计/DiffStats 皆为噪声）。
func TestPairChunks_DuplicateContentGreedy(t *testing.T) {
	old := oldFlat("X", "X", "Y")
	newC := newFlat("X", "X", "X")
	plan := PairChunksForDiff(old, newC)
	assertPlan(t, plan, 2, 1, 1) // 两块 X 配对、第三块 X added；旧 Y deleted
	assertKeptNewIdxAsc(t, plan)
	if plan.Kept[0].OldID != rowID(0) || plan.Kept[1].OldID != rowID(1) {
		t.Fatalf("greedy order violated: %+v", plan.Kept)
	}
}

// ⑤ 核心收益钉：删除一块导致后续全部位次漂移——按内容配对必须保住
// 漂移块（按位次旧方案会全删重建，diff-commit 的经济性全赖此钉）。
func TestPairChunks_DeleteBlockShiftsSuffix(t *testing.T) {
	old := oldFlat("A", "B", "C", "D")
	newC := newFlat("A", "C", "D", "E")
	plan := PairChunksForDiff(old, newC)
	assertPlan(t, plan, 3, 1, 1)
	assertKeptNewIdxAsc(t, plan)
	if plan.Deleted[0] != rowID(1) { // B
		t.Fatalf("deleted mismatch: got %s want %s", plan.Deleted[0], rowID(1))
	}
	keptIDs := map[string]bool{}
	for _, p := range plan.Kept {
		keptIDs[p.OldID] = true
	}
	if !keptIDs[rowID(0)] || !keptIDs[rowID(2)] || !keptIDs[rowID(3)] {
		t.Fatalf("shifted rows not kept: %v", keptIDs)
	}
}

// ⑥ 父子约束·父保留+一子内容变：变子 added+其旧行孤儿，其余子保留；
// 子配对受父筛选——即使另一父下有同 content 子块也不得越父配对。
// 交织切片形状（生产 SplitParentChild 输出序）：[父0,子…,父1,子…]，
// ParentIdx=父序号 0/1（非切片索引——序号语义钉本身即在此夹具生效）。
func TestPairChunks_ParentKeptChildChanged(t *testing.T) {
	old := []PairingOldRow{
		{ID: "p0", Content: "P", ChunkIndex: 0, Role: "parent"},
		{ID: "c0", Content: "ca", ChunkIndex: 0, Role: "child", ParentChunkID: "p0"},
		{ID: "c1", Content: "cb", ChunkIndex: 1, Role: "child", ParentChunkID: "p0"},
		// 第二父同内容（独立 ID），其下有与 c0 同文的子——筛选钉的诱饵。
		{ID: "p1", Content: "P", ChunkIndex: 1, Role: "parent"},
		{ID: "c2", Content: "ca", ChunkIndex: 0, Role: "child", ParentChunkID: "p1"},
	}
	newC := []PairingNewChunk{
		{Content: "P", Role: "parent", ParentIdx: -1}, // 0
		{Content: "ca2", Role: "child", ParentIdx: 0}, // 1 变子（父序号 0）
		{Content: "cb", Role: "child", ParentIdx: 0},  // 2
		{Content: "P", Role: "parent", ParentIdx: -1}, // 3 父序号 1
		{Content: "ca", Role: "child", ParentIdx: 1},  // 4（父序号 1——切片索引 3 语义判误者在此被钉）
	}
	plan := PairChunksForDiff(old, newC)
	// p0↔new0、c1↔new2、p1↔new3、c2↔new4 保留；new1 added；c0 孤儿。
	assertPlan(t, plan, 4, 1, 1)
	assertKeptNewIdxAsc(t, plan)
	if plan.Deleted[0] != "c0" {
		t.Fatalf("orphan mismatch: got %s want c0", plan.Deleted[0])
	}
	// 筛选钉：new4 的 ca 必须配到 p1 名下的 c2，不得越父配到 c0。
	for _, p := range plan.Kept {
		if p.NewIdx == 4 && p.OldID != "c2" {
			t.Fatalf("cross-parent pairing: new4 -> %s want c2", p.OldID)
		}
	}
	// Kept 序承诺活钉：混合 pass 产出 [0,2,3,4] 必须已升序。
	wantIdx := []int{0, 2, 3, 4}
	for i, idx := range wantIdx {
		if plan.Kept[i].NewIdx != idx {
			t.Fatalf("kept global order wrong: got %+v", plan.Kept)
		}
	}
}

// ⑦ 父死子随：父内容变（父双位 added/deleted）→ 其全部新子强制 added、
// 旧子行入孤儿集，即使子内容一字未变。
func TestPairChunks_ParentDiesChildrenFollow(t *testing.T) {
	old := []PairingOldRow{
		{ID: "p0", Content: "P", ChunkIndex: 0, Role: "parent"},
		{ID: "c0", Content: "ca", ChunkIndex: 0, Role: "child", ParentChunkID: "p0"},
		{ID: "c1", Content: "cb", ChunkIndex: 1, Role: "child", ParentChunkID: "p0"},
	}
	newC := []PairingNewChunk{
		{Content: "P-new", Role: "parent", ParentIdx: -1}, // 父变
		{Content: "ca", Role: "child", ParentIdx: 0},      // 子文未变仍陪葬
		{Content: "cb", Role: "child", ParentIdx: 0},
	}
	plan := PairChunksForDiff(old, newC)
	assertPlan(t, plan, 0, 3, 3)
	del := map[string]bool{}
	for _, id := range plan.Deleted {
		del[id] = true
	}
	if !del["p0"] || !del["c0"] || !del["c1"] {
		t.Fatalf("parent-death orphan set incomplete: %v", plan.Deleted)
	}
}

// ⑦b 父 kept 而新侧整节子清零：旧子全孤儿（对称面钉——父活不救子）。
func TestPairChunks_ParentKeptAllChildrenDrop(t *testing.T) {
	old := []PairingOldRow{
		{ID: "p0", Content: "P", ChunkIndex: 0, Role: "parent"},
		{ID: "c0", Content: "ca", ChunkIndex: 0, Role: "child", ParentChunkID: "p0"},
		{ID: "c1", Content: "cb", ChunkIndex: 1, Role: "child", ParentChunkID: "p0"},
	}
	newC := []PairingNewChunk{
		{Content: "P", Role: "parent", ParentIdx: -1}, // 父文不变、新侧无子
	}
	plan := PairChunksForDiff(old, newC)
	assertPlan(t, plan, 1, 0, 2)
	assertKeptNewIdxAsc(t, plan)
	if plan.Kept[0].OldID != "p0" {
		t.Fatalf("parent pairing mismatch: %+v", plan.Kept[0])
	}
	del := map[string]bool{}
	for _, id := range plan.Deleted {
		del[id] = true
	}
	if !del["c0"] || !del["c1"] {
		t.Fatalf("dropped children not orphared: %v", plan.Deleted)
	}
}

// ⑦c 对称父死子随（CON-16/DIS-4 决策钉）：策略降级 parent_child→flat——
// flat 新子块的收养对象仅 ParentChunkID=空串 的孤儿旧行；旧 parented 子行
// 与其父母全入孤儿集（悬空父指针从一开始就结构上不可能）。
func TestPairChunks_FlatPathRejectsParentedOldRows(t *testing.T) {
	old := []PairingOldRow{
		{ID: "p0", Content: "P", ChunkIndex: 0, Role: "parent"},
		{ID: "c0", Content: "ca", ChunkIndex: 0, Role: "child", ParentChunkID: "p0"},
		{ID: "c1", Content: "flat", ChunkIndex: 1, Role: "child"}, // 孤儿旧子
	}
	// 配置切 flat 后重嵌：新子 flat 声明，与旧 parented 子同文。
	newC := []PairingNewChunk{
		{Content: "ca", Role: "child", ParentIdx: -1},
		{Content: "flat", Role: "child", ParentIdx: -1},
	}
	plan := PairChunksForDiff(old, newC)
	assertPlan(t, plan, 1, 1, 2) // 仅孤儿 c1 被收养；p0+c0 入孤儿集；ca 新块 added
	assertKeptNewIdxAsc(t, plan)
	if plan.Kept[0].OldID != "c1" {
		t.Fatalf("flat adoption must reject parented old rows: %+v", plan.Kept)
	}
	del := map[string]bool{}
	for _, id := range plan.Deleted {
		del[id] = true
	}
	if !del["p0"] || !del["c0"] {
		t.Fatalf("downgraded parented rows must follow parent to orphans: %v", plan.Deleted)
	}
}

// ⑦d 同父下重复子块的位次确定性钉（CON-18 缺口）：生产编号形状（父
// chunk_index 0..P-1 / 子 0..C-1 命名空间重叠）下同文双子按升序收养。
func TestPairChunks_DuplicateChildrenSameParentOrdinal(t *testing.T) {
	old := []PairingOldRow{
		{ID: "p0", Content: "P", ChunkIndex: 0, Role: "parent"},
		{ID: "c0", Content: "ca", ChunkIndex: 0, Role: "child", ParentChunkID: "p0"},
		{ID: "c1", Content: "ca", ChunkIndex: 1, Role: "child", ParentChunkID: "p0"},
	}
	newC := []PairingNewChunk{
		{Content: "P", Role: "parent", ParentIdx: -1}, // 0
		{Content: "ca", Role: "child", ParentIdx: 0},  // 1
		{Content: "ca", Role: "child", ParentIdx: 0},  // 2
		{Content: "cb", Role: "child", ParentIdx: 0},  // 3 added
	}
	plan := PairChunksForDiff(old, newC)
	assertPlan(t, plan, 3, 1, 0)
	assertKeptNewIdxAsc(t, plan)
	if plan.Kept[1].OldID != "c0" || plan.Kept[2].OldID != "c1" {
		t.Fatalf("same-parent duplicate ordinal violated: %+v", plan.Kept)
	}
}

// ⑧ 角色隔离：内容与某旧 parent 全同但角色标 child 的新块不得配对（误配钉）。
func TestPairChunks_RoleIsolation(t *testing.T) {
	old := []PairingOldRow{
		{ID: "p0", Content: "SAME", ChunkIndex: 0, Role: "parent"},
	}
	// flat 侧视角声明 child（角色误配形态）。
	newC := []PairingNewChunk{
		{Content: "SAME", Role: "child", ParentIdx: -1},
	}
	plan := PairChunksForDiff(old, newC)
	assertPlan(t, plan, 0, 1, 1)
}

// ⑨ 空集边界：新文档（旧空）全 added；文档清空（新空）全 deleted。
func TestPairChunks_EmptySides(t *testing.T) {
	plan := PairChunksForDiff(nil, newFlat("A", "B"))
	assertPlan(t, plan, 0, 2, 0)
	plan = PairChunksForDiff(oldFlat("A", "B"), nil)
	assertPlan(t, plan, 0, 0, 2)
}

// ⑩ 父序号越界容退：ParentIdx ≥ 父数（零父声明集的 0 亦同）按 flat 口径
// 处理（数据异常不崩、不构成配对阻塞；ordinal 语义下「指向 child 行」形态
// 结构性不可达，越界钉即容退全集钉）。
func TestPairChunks_AnomalousParentRefDegradesFlat(t *testing.T) {
	for _, idx := range []int{0, 1, 7} {
		old := oldFlat("A")
		newC := []PairingNewChunk{
			{Content: "A", Role: "child", ParentIdx: idx}, // 新集零父——任何序号皆越界
		}
		plan := PairChunksForDiff(old, newC)
		assertPlan(t, plan, 1, 0, 0)
		assertKeptNewIdxAsc(t, plan)
	}
	// 有父但序号越界：父1 存在、声明序号 2。
	old := []PairingOldRow{
		{ID: "p0", Content: "P", ChunkIndex: 0, Role: "parent"},
		{ID: "c0", Content: "ca", ChunkIndex: 0, Role: "child", ParentChunkID: "p0"},
	}
	newC := []PairingNewChunk{
		{Content: "P", Role: "parent", ParentIdx: -1},
		{Content: "ca", Role: "child", ParentIdx: 2},
	}
	plan := PairChunksForDiff(old, newC)
	// 容退 flat 只收养孤儿旧行——c0 是 parented 行，拒绝收养（对称钉联动）：
	// 父 kept、子 added、旧 c0 随（父未死但子被孤立——flat 容退即孤儿化语义）。
	assertPlan(t, plan, 1, 1, 1)
	assertKeptNewIdxAsc(t, plan)
	if !strings.HasPrefix(plan.Kept[0].OldID, "p0") || plan.Deleted[0] != "c0" {
		t.Fatalf("degrade-flat must not adopt parented row: %+v", plan)
	}
}
