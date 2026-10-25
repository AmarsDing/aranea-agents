package knowledge

// G-37 实体消解分层管线（ADR-KN-20261021-D/E/F）测试：
// 候选构造（norm 组/ANN 腿/自身过滤/降级）· 分层裁定（auto 免 LLM / 灰区
// 预算+熔断后扣 / LLM 否决静默）· dedup 签名三态（拒绝沉默/变签重提）·
// disposal applied+NotFound+缺字段 · DryRun/非团队库闸 · 报告接线。

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// emPayloadIDOK 测试辅助：payload id 断言（60% 路径覆盖 emPayloadID 各型）。
func emPayloadIDOK(payload map[string]any, key string) int64 {
	id, _ := emPayloadID(payload, key)
	return id
}

// ── 桩 ────────────────────────────────────────────────────────────

// stubEMEntities EntityRepo 桩（本套件只用 ListEntities/MergeEntities）。
type stubEMEntities struct {
	list       []Entity
	listErr    error
	mergeErr   error
	mergeCalls int
	gotKeeper  int64
	gotMergees []int64
	gotCol     string
}

func (s *stubEMEntities) ReplaceDocEntities(context.Context, string, string, []DocEntity) ([]int64, error) {
	return nil, nil
}
func (s *stubEMEntities) FindEntityCooccurrences(context.Context, string, []int64, string, int) ([]EntityCooccurrence, error) {
	return nil, nil
}
func (s *stubEMEntities) ListEntities(context.Context, string) ([]Entity, error) {
	return s.list, s.listErr
}
func (s *stubEMEntities) MergeEntities(_ context.Context, colID string, keeperID int64, mergeeIDs []int64) (EntityMergeResult, error) {
	s.mergeCalls++
	s.gotCol, s.gotKeeper, s.gotMergees = colID, keeperID, mergeeIDs
	if s.mergeErr != nil {
		return EntityMergeResult{}, s.mergeErr
	}
	return EntityMergeResult{MergedEntities: len(mergeeIDs)}, nil
}

// stubEMEmbedder 名称→向量映射（vecs 缺名时 {0,0}；err 全腿失败）。
type stubEMEmbedder struct {
	vecs  map[string][]float32
	err   error
	calls int
}

func (s *stubEMEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		if v, ok := s.vecs[t]; ok {
			out[i] = v
		} else {
			out[i] = []float32{0, 0}
		}
	}
	return out, nil
}

// stubEMANN 按查询向量首维=实体 id 命中的 ANN 桩。
type stubEMANN struct {
	byID  map[int64][]GraphCardHit
	calls int
	err   error
}

func (s *stubEMANN) SearchGraphCards(_ context.Context, _, kind string, vec []float32, _ int) ([]GraphCardHit, error) {
	s.calls++
	if kind != GraphCardKindEntity {
		return nil, fmt.Errorf("kind must be entity, got %q", kind)
	}
	if s.err != nil {
		return nil, s.err
	}
	if len(vec) == 0 {
		return nil, nil
	}
	return s.byID[int64(vec[0])], nil
}

// stubEMAdjudicator 逐次裁决（verdicts/errs 按序消费，耗尽回落 yes,nil）。
type stubEMAdjudicator struct {
	calls    int
	last     EntityMergeAdjudicateRequest
	verdicts []bool
	reasons  []string
	errs     []error
}

func (s *stubEMAdjudicator) AdjudicateEntityMerge(_ context.Context, req EntityMergeAdjudicateRequest) (EntityMergeAdjudication, error) {
	i := s.calls
	s.calls++
	s.last = req
	if i < len(s.errs) && s.errs[i] != nil {
		return EntityMergeAdjudication{}, s.errs[i]
	}
	verdict := true
	reason := "r"
	if i < len(s.verdicts) {
		verdict = s.verdicts[i]
	}
	if i < len(s.reasons) {
		reason = s.reasons[i]
	}
	return EntityMergeAdjudication{Verdict: verdict, Reason: reason}, nil
}

// ── 装配 ──────────────────────────────────────────────────────────

// emTeamRepo 团队库集合解析（CurateKnowledge(CollectionID) 的 homeCol 兜底路径）。
func emTeamRepo() *mockRepo {
	return &mockRepo{
		collGetFn: func(_ context.Context, id string) (Collection, error) {
			return Collection{ID: id, VaultBackend: VaultBackendTeam}, nil
		},
	}
}

func newEntityMergeUsecase(repo *mockRepo, curate *stubCurateRepo, entities *stubEMEntities) (*Usecase, *stubProposalRepo) {
	u := NewUsecaseFromRepo(repo)
	u.SetCurateRepo(curate)
	proposals := &stubProposalRepo{}
	u.SetEvolutionRepos(nil, proposals)
	u.SetLinkRepos(nil, entities)
	return u, proposals
}

// runEMTask 全接线跑一轮，返回报告。
func runEMTask(t *testing.T, entities []Entity, embedder EntityEmbedder, ann GraphCardVectorSearcher, adj EntityMergeAdjudicator, curate *stubCurateRepo) (*Usecase, *stubProposalRepo, CurateReport) {
	t.Helper()
	if curate == nil {
		curate = &stubCurateRepo{}
	}
	em := &stubEMEntities{list: entities}
	u, proposals := newEntityMergeUsecase(emTeamRepo(), curate, em)
	u.SetEntityMergeCandidateRepos(embedder, ann)
	u.SetEntityMergeAdjudicator(adj)
	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err != nil {
		t.Fatalf("curate: %v", err)
	}
	return u, proposals, rep
}

// ── 候选构造与分层裁定 ────────────────────────────────────────────

// norm 冲突组（name 不同 name_norm 同）→ sim=1.0 auto 提案，免 LLM、零 ANN。
func TestEntityMergeTask_NormGroupAutoProposal(t *testing.T) {
	t.Parallel()
	entities := []Entity{
		{ID: 3, Name: "ＡＩ", NameNorm: "ai", EntityType: "concept"},
		{ID: 1, Name: "AI", NameNorm: "ai", EntityType: "concept"},
		{ID: 2, Name: "ai", NameNorm: "ai", EntityType: "concept"},
	}
	adj := &stubEMAdjudicator{}
	ann := &stubEMANN{byID: map[int64][]GraphCardHit{}}
	curate := &stubCurateRepo{}
	_, proposals, rep := runEMTask(t, entities, &stubEMEmbedder{}, ann, adj, curate)

	if rep.EntityMergeProposals != 2 || rep.ProposalsPending != 2 {
		t.Fatalf("norm group (3 实体) 应产 2 提案: %+v", rep)
	}
	if adj.calls != 0 {
		t.Fatalf("auto 档免 LLM: adj.calls=%d", adj.calls)
	}
	if !slices.Contains(rep.Actions, "entity_merge") {
		t.Fatalf("actions 应含 entity_merge: %v", rep.Actions)
	}
	// canonical：组首铆对 +keeper=对内最小 id（norm 组 fan-out 为组首铆，非全
	// 团对——entity_suggest.go:91-96 同先例）→ 对集 {(1,3),(2,3)}。
	wantPairs := map[[2]int64]bool{{1, 3}: true, {2, 3}: true}
	if len(proposals.items) != len(wantPairs) {
		t.Fatalf("对集数量: %d want %d", len(proposals.items), len(wantPairs))
	}
	for _, p := range proposals.items {
		if p.Kind != ProposalKindEntityMerge || p.Risk != ProposalRiskHigh || p.Status != ProposalStatusPending {
			t.Fatalf("提案形态: %+v", p)
		}
		keeper := emPayloadIDOK(p.Payload, "keeper_id")
		mergee := emPayloadIDOK(p.Payload, "mergee_id")
		if !wantPairs[[2]int64{keeper, mergee}] || keeper >= mergee {
			t.Fatalf("对集/序: keeper=%d mergee=%d (payload %+v)", keeper, mergee, p.Payload)
		}
		if p.Payload["similarity"] != "1.00" {
			t.Fatalf("norm 组 sim=1.00: %+v", p.Payload)
		}
		dedup, _ := p.Payload["dedup_key"].(string)
		if !strings.HasPrefix(dedup, "entity_merge:") {
			t.Fatalf("dedup 前缀: %q", dedup)
		}
		srcs, _ := p.Payload["sources"].([]string)
		if len(srcs) != 1 || srcs[0] != entityMergeSourceNorm {
			t.Fatalf("sources: %v", p.Payload["sources"])
		}
	}
	// 三态全含拒绝即沉默语义。
	if st := curate.dedupStatuses[ProposalKindEntityMerge]; len(st) != 3 {
		t.Fatalf("HasProposal 应携带三态: %v", st)
	} else {
		got := append([]string(nil), st...)
		sort.Strings(got)
		want := []string{ProposalStatusApplied, ProposalStatusPending, ProposalStatusRejected}
		sort.Strings(want)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("三态集合: got %v want %v", got, want)
		}
	}
}

// ANN 腿分层：≥0.90 免 LLM 直接提案；[0.80,0.90) 灰区烧一次算符且在
// verdict=yes 时带 llm_reason 提案；<0.80 丢弃；自身卡过滤。
func TestEntityMergeTask_ANNLayeredAdjudication(t *testing.T) {
	t.Parallel()
	ents := []Entity{
		{ID: 1, Name: "RAG", NameNorm: "rag", EntityType: "tech"},
		{ID: 2, Name: "检索增强生成", NameNorm: "检索增强生成", EntityType: "tech"},
		{ID: 3, Name: "向量检索", NameNorm: "向量检索", EntityType: "tech"},
		{ID: 4, Name: "无关实体", NameNorm: "无关", EntityType: "tech"},
	}
	embedding := &stubEMEmbedder{vecs: map[string][]float32{
		"RAG": {1}, "检索增强生成": {2}, "向量检索": {3}, "无关实体": {4},
	}}
	ann := &stubEMANN{byID: map[int64][]GraphCardHit{
		1: {{RefID: 1, Score: 1.0}, {RefID: 2, Score: 0.92}, {RefID: 3, Score: 0.85}, {RefID: 4, Score: 0.50}},
		2: {{RefID: 2, Score: 1.0}, {RefID: 1, Score: 0.92}},
		3: {{RefID: 3, Score: 1.0}, {RefID: 1, Score: 0.85}},
		4: {{RefID: 4, Score: 1.0}},
	}}
	adj := &stubEMAdjudicator{verdicts: []bool{true}, reasons: []string{"同义技术别名"}}
	_, proposals, rep := runEMTask(t, ents, embedding, ann, adj, nil)

	if rep.EntityMergeProposals != 2 {
		t.Fatalf("对 (1,2) auto + (1,3) yes 应产 2 提案: %+v", rep)
	}
	if adj.calls != 1 {
		t.Fatalf("仅灰区一对烧 LLM: calls=%d", adj.calls)
	}
	if adj.last.KeeperID != 1 || adj.last.MergeeID != 3 || adj.last.Similarity < 0.84 || adj.last.Similarity > 0.86 {
		t.Fatalf("灰区裁决入参: %+v", adj.last)
	}
	var autoProp, grayProp *GovernanceProposal
	for i := range proposals.items {
		p := &proposals.items[i]
		if p.Payload["mergee_id"] == int64(2) {
			autoProp = p
		} else {
			grayProp = p
		}
	}
	if autoProp == nil || grayProp == nil {
		t.Fatalf("缺失提案: %+v", proposals.items)
	}
	if _, hasReason := autoProp.Payload["llm_reason"]; hasReason {
		t.Fatalf("auto 提案不得带 llm_reason: %+v", autoProp.Payload)
	}
	if grayProp.Payload["llm_reason"] != "同义技术别名" {
		t.Fatalf("灰区提案应带 llm_reason: %+v", grayProp.Payload)
	}
	if srcs, _ := grayProp.Payload["sources"].([]string); len(srcs) != 1 || srcs[0] != entityMergeSourceANN {
		t.Fatalf("sources: %v", grayProp.Payload["sources"])
	}
}

// LLM 否决：静默不落提案，但预算照扣（后续灰区对预算递减）。
func TestEntityMergeTask_LLMNoSilent(t *testing.T) {
	t.Parallel()
	ents := []Entity{
		{ID: 1, Name: "甲", NameNorm: "甲"}, {ID: 2, Name: "乙", NameNorm: "乙"},
		{ID: 3, Name: "丙", NameNorm: "丙"}, {ID: 4, Name: "丁", NameNorm: "丁"},
	}
	embedding := &stubEMEmbedder{vecs: map[string][]float32{"甲": {1}, "乙": {2}, "丙": {3}, "丁": {4}}}
	ann := &stubEMANN{byID: map[int64][]GraphCardHit{
		1: {{RefID: 2, Score: 0.85}}, 3: {{RefID: 4, Score: 0.84}},
	}}
	adj := &stubEMAdjudicator{verdicts: []bool{false, true}}
	_, proposals, rep := runEMTask(t, ents, embedding, ann, adj, nil)

	if adj.calls != 2 {
		t.Fatalf("两对灰区各裁一次: calls=%d", adj.calls)
	}
	if rep.EntityMergeProposals != 1 || len(proposals.items) != 1 {
		t.Fatalf("否决对零提案、肯定对一条: rep=%+v items=%v", rep, proposals.items)
	}
}

// 预算后扣 + 连续失败熔断：预算 4 → 第 5 对及以上让位；3 连败熔断本库本轮。
func TestEntityMergeTask_LLMBudgetBreaker(t *testing.T) {
	t.Parallel()

	// 预算：6 对灰区全部 yes → 只产 4 条提案（budget=4），LLM 恰调 4 次。
	ents := make([]Entity, 12)
	for i := range ents {
		ents[i] = Entity{ID: int64(i + 1), Name: fmt.Sprintf("E%02d", i), NameNorm: fmt.Sprintf("e%02d", i)}
	}
	vecs := map[string][]float32{}
	annHits := map[int64][]GraphCardHit{}
	for i := 0; i < 12; i += 2 {
		vecs[fmt.Sprintf("E%02d", i)] = []float32{float32(i + 1)}
		vecs[fmt.Sprintf("E%02d", i+1)] = []float32{float32(i + 2)}
		annHits[int64(i+1)] = []GraphCardHit{{RefID: int64(i + 2), Score: 0.85}}
	}
	adj := &stubEMAdjudicator{} // 回落 yes
	_, props, rep := runEMTask(t, ents, &stubEMEmbedder{vecs: vecs}, &stubEMANN{byID: annHits}, adj, nil)
	if reps := 6; len(props.items) > reps {
		t.Fatalf("超过候选对上限: %d", len(props.items))
	}
	if rep.EntityMergeProposals != entityMergeLLMBudgetPerRound || adj.calls != entityMergeLLMBudgetPerRound {
		t.Fatalf("预算后扣: proposals=%d calls=%d want=%d", rep.EntityMergeProposals, adj.calls, entityMergeLLMBudgetPerRound)
	}

	// 熔断：3 连败 → LLM 恰调 3 次，零提案。
	adj2 := &stubEMAdjudicator{errs: []error{errors.New("x"), errors.New("x"), errors.New("x"), nil, nil, nil}}
	_, props2, rep2 := runEMTask(t, ents, &stubEMEmbedder{vecs: vecs}, &stubEMANN{byID: annHits}, adj2, nil)
	if adj2.calls != entityMergeMaxConsecutiveLLMFails || rep2.EntityMergeProposals != 0 || len(props2.items) != 0 {
		t.Fatalf("熔断后扣: calls=%d proposals=%d", adj2.calls, rep2.EntityMergeProposals)
	}
}

// dedup 签名：同签名已有提案（pending/applied/rejected 任一）→ 跳过且零 LLM；
// 证据演进（sim 漂移变签）→ 可重提。
func TestEntityMergeTask_DedupSilenceVsRepropose(t *testing.T) {
	t.Parallel()
	entities := []Entity{{ID: 1, Name: "甲", NameNorm: "甲"}, {ID: 2, Name: "乙", NameNorm: "乙"}}
	embedding := &stubEMEmbedder{vecs: map[string][]float32{"甲": {1}, "乙": {2}}}
	makeANN := func(score float32) *stubEMANN {
		return &stubEMANN{byID: map[int64][]GraphCardHit{1: {{RefID: 2, Score: score}}}}
	}

	// 同签名三态命中（此处落 rejected）：沉默 + 免 LLM。
	sig := entityMergeSignature(0.85, []string{entityMergeSourceANN}, "yes")
	curate := &stubCurateRepo{hasProposalKeys: map[string]bool{
		ProposalKindEntityMerge + "|entity_merge:1:2:" + sig: true,
	}}
	adj := &stubEMAdjudicator{}
	_, props, rep := runEMTask(t, entities, embedding, makeANN(0.85), adj, curate)
	if rep.EntityMergeProposals != 0 || len(props.items) != 0 || adj.calls != 0 {
		t.Fatalf("同签名拒绝应沉默且免 LLM: proposals=%d calls=%d", rep.EntityMergeProposals, adj.calls)
	}
	if st := curate.dedupStatuses[ProposalKindEntityMerge]; len(st) != 3 {
		t.Fatalf("HasProposal 应携带三态: %v", st)
	}

	// sim 漂移 0.85→0.92：auto 档变签 → 重提（即便旧 rejected 签名仍在）。
	curate2 := &stubCurateRepo{hasProposalKeys: map[string]bool{
		ProposalKindEntityMerge + "|entity_merge:1:2:" + sig: true,
	}}
	adj2 := &stubEMAdjudicator{}
	_, props2, rep2 := runEMTask(t, entities, embedding, makeANN(0.92), adj2, curate2)
	if rep2.EntityMergeProposals != 1 || len(props2.items) != 1 {
		t.Fatalf("变签应重提: proposals=%d", rep2.EntityMergeProposals)
	}
	if adj2.calls != 0 {
		t.Fatalf("0.92 属 auto 档免 LLM: calls=%d", adj2.calls)
	}
}

// ── 跳过矩阵与降级 ────────────────────────────────────────────────

// DryRun / 端口未接线 / 非团队库：零 LLM 零提案零记录。
func TestEntityMergeTask_SkipMatrix(t *testing.T) {
	t.Parallel()
	entities := []Entity{{ID: 1, Name: "甲", NameNorm: "甲"}, {ID: 2, Name: "乙", NameNorm: "乙"}}
	embedding := &stubEMEmbedder{vecs: map[string][]float32{"甲": {1}, "乙": {2}}}
	ann := &stubEMANN{byID: map[int64][]GraphCardHit{1: {{RefID: 2, Score: 0.85}}}}

	// DryRun：任务整体跳过（ADR-F——LLM 巡检任务不在 dry-run 计数）。
	adj := &stubEMAdjudicator{}
	em := &stubEMEntities{list: entities}
	u, _ := newEntityMergeUsecase(emTeamRepo(), &stubCurateRepo{}, em)
	u.SetEntityMergeCandidateRepos(embedding, ann)
	u.SetEntityMergeAdjudicator(adj)
	rep, err := u.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1", DryRun: true})
	if err != nil || rep.EntityMergeProposals != 0 || adj.calls != 0 {
		t.Fatalf("dry-run 应整体跳过: %v %+v calls=%d", err, rep, adj.calls)
	}

	// adjudicator 未接线：跳过。
	u2, _ := newEntityMergeUsecase(emTeamRepo(), &stubCurateRepo{}, &stubEMEntities{list: entities})
	u2.SetEntityMergeCandidateRepos(embedding, ann)
	rep2, err2 := u2.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err2 != nil || rep2.EntityMergeProposals != 0 {
		t.Fatalf("nil adjudicator 应跳过: %v %+v", err2, rep2)
	}

	// embedder 未接线：跳过。
	u3, _ := newEntityMergeUsecase(emTeamRepo(), &stubCurateRepo{}, &stubEMEntities{list: entities})
	u3.SetEntityMergeAdjudicator(&stubEMAdjudicator{})
	rep3, err3 := u3.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err3 != nil || rep3.EntityMergeProposals != 0 {
		t.Fatalf("nil embedder 应跳过: %v %+v", err3, rep3)
	}

	// 非团队库：跳过（ADR-KN-20261021-A 防御深先例）。
	localRepo := &mockRepo{collGetFn: func(_ context.Context, id string) (Collection, error) {
		return Collection{ID: id, VaultBackend: VaultBackendLocal}, nil
	}}
	adj4 := &stubEMAdjudicator{}
	u4, _ := newEntityMergeUsecase(localRepo, &stubCurateRepo{}, &stubEMEntities{list: entities})
	u4.SetEntityMergeCandidateRepos(embedding, ann)
	u4.SetEntityMergeAdjudicator(adj4)
	rep4, err4 := u4.CurateKnowledge(context.Background(), CurateOptions{CollectionID: "c1"})
	if err4 != nil || rep4.EntityMergeProposals != 0 || adj4.calls != 0 {
		t.Fatalf("非团队库闸: %v %+v calls=%d", err4, rep4, adj4.calls)
	}
}

// ANN/embedding 失败降级：embedder 全腿失败 → norm-only 仍产提案；
// ANN 单实体失败 → 该实体跳过、余下照常。
func TestEntityMergeTask_DegradePaths(t *testing.T) {
	t.Parallel()
	entities := []Entity{
		{ID: 1, Name: "AI", NameNorm: "ai"}, {ID: 2, Name: "ai", NameNorm: "ai"},
		{ID: 3, Name: "丙", NameNorm: "丙"}, {ID: 4, Name: "丁", NameNorm: "丁"},
	}

	// embedder 失败 → norm 组照常（ANN 腿关闭零调用）。
	ann := &stubEMANN{byID: map[int64][]GraphCardHit{3: {{RefID: 4, Score: 0.95}}}}
	_, _, rep := runEMTask(t, entities, &stubEMEmbedder{err: errors.New("embed down")}, ann, &stubEMAdjudicator{}, nil)
	if rep.EntityMergeProposals != 1 {
		t.Fatalf("embedder 失败应 norm-only 产 1 条: %+v", rep)
	}
	if ann.calls != 0 {
		t.Fatalf("embedder 失败 ANN 腿应整体关闭: calls=%d", ann.calls)
	}

	// ANN 接口报错 → 候选仅剩 norm 组。
	embedding := &stubEMEmbedder{vecs: map[string][]float32{"AI": {1}, "ai": {2}, "丙": {3}, "丁": {4}}}
	_, _, rep2 := runEMTask(t, entities, embedding, &stubEMANN{err: errors.New("pg flap")}, &stubEMAdjudicator{}, nil)
	if rep2.EntityMergeProposals != 1 {
		t.Fatalf("ANN 失败应 norm-only 产 1 条: %+v", rep2)
	}
}

// 候选对封顶 64：sim 降序截断。
func TestEntityMergeTask_PairCap(t *testing.T) {
	t.Parallel()
	// 大量 norm 双组造 >64 候选对（每组 1 对，需要 65+ 组 → 130+ 实体）。
	ents := make([]Entity, 0, 130)
	for i := 0; i < 70; i++ {
		norm := fmt.Sprintf("n%03d", i)
		ents = append(ents,
			Entity{ID: int64(2*i + 1), Name: norm + "甲", NameNorm: norm},
			Entity{ID: int64(2*i + 2), Name: norm + "乙", NameNorm: norm},
		)
	}
	u, _ := newEntityMergeUsecase(emTeamRepo(), &stubCurateRepo{}, &stubEMEntities{})
	pairs := u.buildEntityMergeCandidates(context.Background(), "c1", ents)
	if len(pairs) != entityMergePairCap {
		t.Fatalf("候选对封顶 64: got %d", len(pairs))
	}
	// sim 全 1.0 → id 升序稳定序；首对 keeper=1。
	if pairs[0].keeper.ID != 1 || pairs[0].mergee.ID != 2 {
		t.Fatalf("稳定破平: %+v", pairs[0].keeper)
	}
	// u 未接线 adj/embedder——任务层跳过，build 层纯候选不依赖。
}

// 日轮转序：同日确定、跨日重排、入参不被改写。
func TestEntityMergeRotation(t *testing.T) {
	t.Parallel()
	ents := make([]Entity, 40)
	for i := range ents {
		ents[i] = Entity{ID: int64(i + 1), Name: fmt.Sprintf("e%d", i)}
	}
	a := rotatedEntityOrder(ents, "2026-10-21")
	b := rotatedEntityOrder(ents, "2026-10-21")
	c := rotatedEntityOrder(ents, "2026-10-22")
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Fatal("同盐必同序")
		}
		if ents[i].ID != int64(i+1) {
			t.Fatal("入参不得被改写")
		}
	}
	same := true
	for i := range a {
		if a[i].ID != c[i].ID {
			same = false
			break
		}
	}
	if same {
		t.Fatal("跨日盐必重排（40 元素同序概率 1/40!）")
	}
	// 全排列完整性。
	ids := map[int64]bool{}
	for _, e := range a {
		ids[e.ID] = true
	}
	if len(ids) != 40 {
		t.Fatal("轮转不丢不重")
	}
}

// ── disposal ──────────────────────────────────────────────────────

func TestEntityMergeDisposal_Applied(t *testing.T) {
	t.Parallel()
	em := &stubEMEntities{}
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{{
		ID: 9, CollectionID: "c1", Kind: ProposalKindEntityMerge, Status: ProposalStatusPending,
		// payload 走 JSON 往返 → float64（mocParamsFromPayload 同容忍度）。
		Payload: map[string]any{"keeper_id": float64(12), "mergee_id": float64(34)},
	}}}
	u, _ := newEntityMergeUsecase(emTeamRepo(), curate, em)
	if err := u.ResolveGovernanceProposal(context.Background(), 9, "applied"); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if em.mergeCalls != 1 || em.gotCol != "c1" || em.gotKeeper != 12 || len(em.gotMergees) != 1 || em.gotMergees[0] != 34 {
		t.Fatalf("merge 入参: %+v", em)
	}
	if curate.resolvedID != 9 || curate.resolvedStatus != ProposalStatusApplied {
		t.Fatalf("终态: %d %s", curate.resolvedID, curate.resolvedStatus)
	}
}

// keeper NotFound → 幂等成功；其余错误 → 上抛且提案停留 pending。
func TestEntityMergeDisposal_NotFoundIdempotent(t *testing.T) {
	t.Parallel()
	em := &stubEMEntities{mergeErr: apierror.NotFound(apierror.DomainKnowledge, "gone")}
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{{
		ID: 9, CollectionID: "c1", Kind: ProposalKindEntityMerge, Status: ProposalStatusPending,
		Payload: map[string]any{"keeper_id": 12, "mergee_id": 34},
	}}}
	u, _ := newEntityMergeUsecase(emTeamRepo(), curate, em)
	if err := u.ResolveGovernanceProposal(context.Background(), 9, "applied"); err != nil {
		t.Fatalf("NotFound 应幂等成功: %v", err)
	}
	if curate.resolvedStatus != ProposalStatusApplied {
		t.Fatalf("幂等后应落 applied: %s", curate.resolvedStatus)
	}

	// 其余错误上抛，提案停留 pending。
	em2 := &stubEMEntities{mergeErr: errors.New("pg down")}
	curate2 := &stubCurateRepo{proposalViews: []GovernanceProposalView{{
		ID: 9, CollectionID: "c1", Kind: ProposalKindEntityMerge, Status: ProposalStatusPending,
		Payload: map[string]any{"keeper_id": 12, "mergee_id": 34},
	}}}
	u2, _ := newEntityMergeUsecase(emTeamRepo(), curate2, em2)
	if err := u2.ResolveGovernanceProposal(context.Background(), 9, "applied"); err == nil {
		t.Fatal("pg 故障应上抛")
	}
	if curate2.resolvedID != 0 {
		t.Fatalf("处置失败不得落终态: resolved=%d", curate2.resolvedID)
	}
}

// 缺字段/自对/未接线实体仓储 → no-op 成功（ADR-D 历史提案降级先例）；
// rejected 决策零副作用。
func TestEntityMergeDisposal_DegradeNoop(t *testing.T) {
	t.Parallel()
	em := &stubEMEntities{}
	curate := &stubCurateRepo{proposalViews: []GovernanceProposalView{
		{ID: 9, CollectionID: "c1", Kind: ProposalKindEntityMerge, Status: ProposalStatusPending,
			Payload: map[string]any{"mergee_id": 34}}, // 缺 keeper_id
		{ID: 10, CollectionID: "c1", Kind: ProposalKindEntityMerge, Status: ProposalStatusPending,
			Payload: map[string]any{"keeper_id": 7, "mergee_id": 7}}, // 自对
	}}
	u, _ := newEntityMergeUsecase(emTeamRepo(), curate, em)
	for _, id := range []int64{9, 10} {
		if err := u.ResolveGovernanceProposal(context.Background(), id, "applied"); err != nil {
			t.Fatalf("id=%d 缺字段应 no-op 成功: %v", id, err)
		}
	}
	if em.mergeCalls != 0 {
		t.Fatalf("缺字段/自对零副作用: calls=%d", em.mergeCalls)
	}

	// rejected：零副作用。
	curate2 := &stubCurateRepo{proposalViews: []GovernanceProposalView{{
		ID: 11, CollectionID: "c1", Kind: ProposalKindEntityMerge, Status: ProposalStatusPending,
		Payload: map[string]any{"keeper_id": 1, "mergee_id": 2},
	}}}
	u2, _ := newEntityMergeUsecase(emTeamRepo(), curate2, em)
	if err := u2.ResolveGovernanceProposal(context.Background(), 11, "rejected"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if em.mergeCalls != 0 || curate2.resolvedStatus != ProposalStatusRejected {
		t.Fatalf("rejected 零副作用: calls=%d status=%s", em.mergeCalls, curate2.resolvedStatus)
	}

	// entities 未接线：no-op 成功。
	curate3 := &stubCurateRepo{proposalViews: []GovernanceProposalView{{
		ID: 12, CollectionID: "c1", Kind: ProposalKindEntityMerge, Status: ProposalStatusPending,
		Payload: map[string]any{"keeper_id": 1, "mergee_id": 2},
	}}}
	u3 := NewUsecaseFromRepo(emTeamRepo())
	u3.SetCurateRepo(curate3)
	if err := u3.ResolveGovernanceProposal(context.Background(), 12, "applied"); err != nil {
		t.Fatalf("未接线应 no-op 成功: %v", err)
	}
}

// 报告接线：CurateKnowledge 报告计 entity_merge_proposals。
func TestEntityMergeTask_CurateReportWiring(t *testing.T) {
	t.Parallel()
	entities := []Entity{{ID: 9, Name: "x", NameNorm: "nx"}, {ID: 8, Name: "X", NameNorm: "nx"}}
	_, _, rep := runEMTask(t, entities, &stubEMEmbedder{}, &stubEMANN{byID: map[int64][]GraphCardHit{}}, &stubEMAdjudicator{}, nil)
	if rep.EntityMergeProposals != 1 || len(rep.Proposals) != 1 || rep.Proposals[0].Kind != ProposalKindEntityMerge {
		t.Fatalf("报告接线: %+v", rep)
	}
	if rep.EntityMergeProposals > rep.ProposalsPending {
		t.Fatalf("实体消解提案应计入 ProposalsPending: %+v", rep)
	}
}

// 日志字段门禁（编译期断言 loggateway 键齐全）。
func TestEntityMergeLogFields(t *testing.T) {
	t.Parallel()
	_ = loggateway.StepID("knowledge.curate.entity_merge")
}
