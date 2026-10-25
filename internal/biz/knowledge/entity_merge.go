package knowledge

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── G-37 实体消解分层管线（ADR-KN-20261021-D/E/F）───────────────────────────
//
// 现状缺口（调研 G-37）：ListEntityMergeSuggestions 在 RPC 内实时整库枚举
// O(N²) 余弦（截 500），阈值硬编码、零持久化；裁决直执绕开提案二审。
// ADR-D：裁决提案化——kind=entity_merge 高风险 pending 提案；applied 处置 =
// EntityRepo.MergeEntities 四层事务（幂等：mergee 不在跳过、keeper NotFound
// 视为幂等成功）；dedup_key 带证据签名三态全含（拒绝即沉默，证据演进可重提）。
// ADR-E：ANN/词法预对齐替 O(N²)——日轮转扫描 ≤entityMergeScanLimit 实体 +
// 批量 embedding + 实体卡 ANN（kind=entity，过取 1 滤自身）+ norm 冲突组；
// 日日 md5(id‖daySalt) 确定序轮转防顽固对独占槽位；ANN 空/卡缺降级 norm-only。
// ADR-F：LLM 只裁 [0.80,0.90) 灰区（≥0.90 免 LLM 直接提案）；判定失败
// fail-close=no 静默不落提案；LLM 预算成功后扣 + 3 连熔断（moc 先例全镜像）。
// DryRun/非团队库/端口未接线一律静默跳过。

const (
	// entityMergeScanLimit 每轮参与候选扫描的实体数（日轮转序截取）。
	entityMergeScanLimit = 32
	// entityMergePairCap 单轮候选对总量上限（sim 降序截，防提案风暴）。
	entityMergePairCap = 64
	// entityMergeANNCandidateLimit 每实体 ANN 近邻上限（过取 1 再滤自身——
	// 自身卡 top-1 必命中；沿用 GraphCardSearchKindLimit 同量级）。
	entityMergeANNCandidateLimit = 9
	// entityMergeLLMBudgetPerRound 每库每轮灰区 LLM 裁决上限（成功后扣——
	// 裁决成行即消耗（yes/no 均计）；LLM 失败不占，G-33 预扣饥饿先例）。
	entityMergeLLMBudgetPerRound = 4
	// entityMergeMaxConsecutiveLLMFails LLM 连续失败熔断线（本库本轮）。
	entityMergeMaxConsecutiveLLMFails = 3
	// entityMergeSigLen 证据签名截断长度（mocMemberSigLen 同式）。
	entityMergeSigLen = 10
)

// entityMergeSource* 候选来源（payload.sources；ANN 腿即语义近邻）。
const (
	entityMergeSourceNorm = "norm"
	entityMergeSourceANN  = "ann"
)

// EntityMergeAdjudicateRequest 灰区裁决入参（原名/类型/sim 证据，零全文——
// agent 出口 2048 cap 纪律：payload 摘要+锚点，裁决同理）。
type EntityMergeAdjudicateRequest struct {
	CollectionID string
	KeeperID     int64
	KeeperName   string
	KeeperType   string
	MergeeID     int64
	MergeeName   string
	MergeeType   string
	Similarity   float64
}

// EntityMergeAdjudication 灰区裁决产物。Verdict=false 即否决（不落提案）。
type EntityMergeAdjudication struct {
	Verdict bool
	Reason  string
}

// EntityMergeAdjudicator LLM 灰区裁决端口（ADR-F 窄口；生产 =
// internal/knowledge.entityMergeAdjudicator）。wire 装配注入；nil 时
// runEntityMergeTask 整体跳过（治理其余任务不受影响）。
// Stability:evolving
type EntityMergeAdjudicator interface {
	AdjudicateEntityMerge(ctx context.Context, req EntityMergeAdjudicateRequest) (EntityMergeAdjudication, error)
}

// SetEntityMergeCandidateRepos 接线候选生成依赖（ADR-E；embedder 复用
// entity_suggest 既有 EntityEmbedder 窄口，ann 复用 G-33 卡片 ANN 窄口）。
// nil 语义不对称（评审 cand-1）：embedder nil → 本梯队任务整体静默跳过
// （runEntityMergeTask 入口门槛，ADR-F 未接线不跑）；ann nil → 仅关闭
// ANN 腿，norm 冲突组照常产提案。注意与运行时降级区分：embedder/ANN 的
// 调用失败才按腿降级（本轮仅 norm），与「未接线」是两种口径。
func (u *Usecase) SetEntityMergeCandidateRepos(embedder EntityEmbedder, ann GraphCardVectorSearcher) {
	u.emEmbedder = embedder
	u.emANN = ann
}

// SetEntityMergeAdjudicator 接线 LLM 灰区裁决端口（可选）。
func (u *Usecase) SetEntityMergeAdjudicator(a EntityMergeAdjudicator) {
	u.emAdjudicator = a
}

// emPair 一条合并候选对（keeper=id 较小者，canonical 去重键）。
type emPair struct {
	keeper  Entity
	mergee  Entity
	sim     float64
	sources map[string]struct{}
}

// merge 合并另一候选观测：sim 取最大、sources 并集。
func (p *emPair) merge(other emPair) {
	if other.sim > p.sim {
		p.sim = other.sim
	}
	for s := range other.sources {
		p.sources[s] = struct{}{}
	}
}

// sortedSources sources 排序切片（dedup 签名与 payload 确定序）。
func (p *emPair) sortedSources() []string {
	out := make([]string, 0, len(p.sources))
	for s := range p.sources {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// entityMergeDaySalt 日轮转盐（UTC 日切换——同一日内确定序，跨日换新序）。
func entityMergeDaySalt(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

// rotatedEntityOrder 按 sha1(id‖daySalt) 确定序重排（不改动入参切片）：
// 每日实体轮转，防止少数候选对长期独占扫描槽（无游标状态的轮换担保）。
func rotatedEntityOrder(entities []Entity, daySalt string) []Entity {
	type keyed struct {
		e Entity
		k [20]byte
	}
	rows := make([]keyed, 0, len(entities))
	for _, e := range entities {
		rows = append(rows, keyed{e: e, k: sha1.Sum([]byte(fmt.Sprintf("%d\x1f%s", e.ID, daySalt)))})
	}
	sort.Slice(rows, func(i, j int) bool { return string(rows[i].k[:]) < string(rows[j].k[:]) })
	out := make([]Entity, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.e)
	}
	return out
}

// entityMergeSignature 证据签名（ADR-D）：sim 两位舍入 ∪ sources 排序 ∪
// 裁决口径 → sha1 截 10 hex。任一证据演进（sim 漂移/来源集合变化/裁定翻案）
// 即变签 → 拒绝/应用后可重提；同签名三态命中即沉默。
func entityMergeSignature(sim float64, sources []string, verdict string) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("%.2f\x1f%s\x1f%s", sim,
		joinUnitSeparator(sources), verdict)))
	return hex.EncodeToString(sum[:])[:entityMergeSigLen]
}

// joinUnitSeparator 单元分隔串接（与 mocMemberSignature 同式）。
func joinUnitSeparator(items []string) string {
	s := ""
	for i, it := range items {
		if i > 0 {
			s += "\x1f"
		}
		s += it
	}
	return s
}

// runEntityMergeTask 实体消解候选 → 分层裁定 → 高风险 pending 提案（curate
// 轮内，ADR-D/E/F）。返回本轮新产提案数；DryRun/未接线/非团队库静默返回 0。
func (u *Usecase) runEntityMergeTask(ctx context.Context, homeCol Collection, colID string, opts CurateOptions, rep *CurateReport) int {
	if opts.DryRun || u.emAdjudicator == nil || u.emEmbedder == nil || u.entities == nil {
		return 0
	}
	// 团队库闸（distill/moc_summary 先例）：local vault 永不巡检裁决；
	// 集合解析失败即跳。
	if homeCol.ID == "" && u.collections != nil {
		col, err := u.collections.GetCollection(ctx, colID)
		if err != nil {
			u.lg.Warn("治理：实体消解集合解析失败（跳过）", loggateway.StepID("knowledge.curate.entity_merge"),
				loggateway.Str("collection_id", colID), loggateway.Err(err))
			return 0
		}
		homeCol = col
	}
	if homeCol.VaultBackend != VaultBackendTeam {
		return 0
	}
	entities, err := u.entities.ListEntities(ctx, homeCol.ID)
	if err != nil {
		u.lg.Warn("治理：实体字典扫描失败（跳过）", loggateway.StepID("knowledge.curate.entity_merge"), loggateway.Err(err))
		return 0
	}
	pairs := u.buildEntityMergeCandidates(ctx, homeCol.ID, entities)
	if len(pairs) == 0 {
		return 0
	}

	filled := 0      // 已消耗 LLM 裁决预算（成功成行才扣）
	produced := 0    // 本轮新产提案数
	consecFails := 0 // LLM 连败计数（熔断口径）
	tripped := false
	allThreeStates := []string{ProposalStatusPending, ProposalStatusApplied, ProposalStatusRejected}
	for _, p := range pairs {
		verdict := "yes"
		if p.sim >= embeddingAutoThreshold {
			verdict = "auto" // ≥0.90 免 LLM 直接提案（ADR-F）
		}
		sources := p.sortedSources()
		dedup := "entity_merge:" + itoa64(p.keeper.ID) + ":" + itoa64(p.mergee.ID) + ":" + entityMergeSignature(p.sim, sources, verdict)
		// 去重三态全含 + 证据签名（ADR-D，ADR-KN-20261021-B 先例）：同签名拒绝即
		// 沉默；证据演进变签可重提。pre-dedup 先行——gray 对已提案不再烧 LLM。
		if u.skipProposal(ctx, homeCol.ID, ProposalKindEntityMerge, dedup, allThreeStates) {
			continue
		}
		if verdict == "yes" {
			// 灰区：预算/熔断让位（零账务，日轮转下轮承接）。
			if filled >= entityMergeLLMBudgetPerRound || tripped {
				continue
			}
			adj, aerr := u.emAdjudicator.AdjudicateEntityMerge(ctx, EntityMergeAdjudicateRequest{
				CollectionID: homeCol.ID,
				KeeperID:     p.keeper.ID, KeeperName: p.keeper.Name, KeeperType: p.keeper.EntityType,
				MergeeID: p.mergee.ID, MergeeName: p.mergee.Name, MergeeType: p.mergee.EntityType,
				Similarity: p.sim,
			})
			if aerr != nil {
				consecFails++
				u.lg.Warn("治理：实体消解 LLM 裁决失败（留下轮）", loggateway.StepID("knowledge.curate.entity_merge"),
					loggateway.Int64("keeper_id", p.keeper.ID), loggateway.Int64("mergee_id", p.mergee.ID), loggateway.Err(aerr))
				if consecFails >= entityMergeMaxConsecutiveLLMFails {
					tripped = true
					u.lg.Warn("治理：实体消解 LLM 连续失败（本库本轮熔断）",
						loggateway.StepID("knowledge.curate.entity_merge"),
						loggateway.Int("consecutive_fails", consecFails),
						loggateway.Str("collection_id", homeCol.ID))
				}
				continue
			}
			consecFails = 0
			filled++ // 裁决成行即消耗（yes/no 同价——后扣纪律）
			if !adj.Verdict {
				continue // LLM 否决：静默不落提案（ADR-F 零噪音）
			}
			recordEntityMergeProposal(u, ctx, homeCol.ID, p, sources, dedup, adj.Reason, rep)
			produced++
			continue
		}
		// auto 档：免 LLM 直接提案。
		recordEntityMergeProposal(u, ctx, homeCol.ID, p, sources, dedup, "", rep)
		produced++
	}
	if produced > 0 {
		rep.Actions = append(rep.Actions, "entity_merge")
	}
	return produced
}

// buildEntityMergeCandidates norm 冲突组 ∪ ANN 近邻候选（ADR-E）。纯候选
// 构造零 LLM 零预算；ANN/embedder 异常一律降级（继续可用腿），不报错出轮。
func (u *Usecase) buildEntityMergeCandidates(ctx context.Context, colID string, entities []Entity) []emPair {
	pairs := map[[2]int64]*emPair{}
	add := func(a, b Entity, sim float64, source string) {
		if a.ID == b.ID {
			return
		}
		if a.ID > b.ID {
			a, b = b, a // canonical：小 id 恒为 keeper（entity_suggest 同纪律）
		}
		key := [2]int64{a.ID, b.ID}
		if existing, ok := pairs[key]; ok {
			existing.merge(emPair{sim: sim, sources: map[string]struct{}{source: {}}})
			return
		}
		pairs[key] = &emPair{keeper: a, mergee: b, sim: sim, sources: map[string]struct{}{source: {}}}
	}

	// norm 冲突组（name 不同 name_norm 相同；entity_suggest 同规则，全局扫描——
	// 迁移期残留稀有，不参与日轮转）。
	byNorm := map[string][]Entity{}
	for _, e := range entities {
		if e.NameNorm != "" {
			byNorm[e.NameNorm] = append(byNorm[e.NameNorm], e)
		}
	}
	for _, g := range byNorm {
		if len(g) < 2 {
			continue
		}
		for _, m := range g[1:] {
			add(g[0], m, 1.0, entityMergeSourceNorm)
		}
	}

	// ANN 预对齐腿：日轮转 ≤scanLimit 实体整批 embed → 逐实体卡近邻。
	if u.emANN != nil && len(entities) >= 2 {
		scanned := rotatedEntityOrder(entities, entityMergeDaySalt(time.Now()))
		if len(scanned) > entityMergeScanLimit {
			scanned = scanned[:entityMergeScanLimit]
		}
		byID := make(map[int64]Entity, len(entities))
		for _, e := range entities {
			byID[e.ID] = e
		}
		names := make([]string, len(scanned))
		for i, e := range scanned {
			names[i] = e.Name
		}
		vecs, verr := u.emEmbedder.Embed(ctx, names)
		if verr != nil || len(vecs) != len(scanned) {
			// expected/got 明带上阵：行数不齐时 verr==nil，Err(nil)=zap.Skip
			// 只剩一行空 Warn（评审 cand-2）。
			u.lg.Warn("治理：实体消解批量 embedding 失败（本轮仅 norm 候选）",
				loggateway.StepID("knowledge.curate.entity_merge"),
				loggateway.Int("expected", len(scanned)), loggateway.Int("got", len(vecs)),
				loggateway.Err(verr))
		} else {
			for i, e := range scanned {
				hits, herr := u.emANN.SearchGraphCards(ctx, colID, GraphCardKindEntity, vecs[i], entityMergeANNCandidateLimit)
				if herr != nil {
					// 单实体 ANN 瞬错：本轮跳过该实体（零计数，下轮日轮转承接）。
					u.lg.Warn("治理：实体消解 ANN 检索失败（该实体本轮跳过）",
						loggateway.StepID("knowledge.curate.entity_merge"),
						loggateway.Int64("entity_id", e.ID), loggateway.Err(herr))
					continue
				}
				for _, h := range hits {
					if h.RefID == e.ID {
						continue // 自身卡 top-1 必命中：过滤
					}
					m, ok := byID[h.RefID]
					if !ok {
						continue
					}
					sim := float64(h.Score)
					if sim < embeddingSuggestThreshold {
						continue
					}
					add(e, m, sim, entityMergeSourceANN)
				}
			}
		}
	}

	// 候选对总量封顶：sim 降序（同分 id 稳定破平），超出下轮承接。
	out := make([]emPair, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].sim != out[j].sim {
			return out[i].sim > out[j].sim
		}
		if out[i].keeper.ID != out[j].keeper.ID {
			return out[i].keeper.ID < out[j].keeper.ID
		}
		return out[i].mergee.ID < out[j].mergee.ID
	})
	if len(out) > entityMergePairCap {
		out = out[:entityMergePairCap]
	}
	return out
}

// recordEntityMergeProposal 落一条 entity_merge 高风险 pending 提案（ADR-D
// payload 平铺+锚点纪律：摘要与 id/名入载荷，全文/长证据不入——agent 出口
// 2048 cap）。DryRun 不会到达本函数——runEntityMergeTask 入口已对 DryRun/
// 未接线静默返回 0（ADR-F），此处不再防御分支（评审 EM-D1）。计数纪律
// （评审 EM-D3）：ProposalsPending 在此递增（主循环绝不覆盖）；
// EntityMergeProposals 由主循环以任务返回值为唯一事实源，本函数不双账。
func recordEntityMergeProposal(u *Usecase, ctx context.Context, colID string, p emPair, sources []string, dedup, llmReason string, rep *CurateReport) {
	sim := fmt.Sprintf("%.2f", p.sim)
	payload := map[string]any{
		"dedup_key":   dedup,
		"keeper_id":   p.keeper.ID,
		"keeper_name": p.keeper.Name,
		"keeper_type": p.keeper.EntityType,
		"mergee_id":   p.mergee.ID,
		"mergee_name": p.mergee.Name,
		"mergee_type": p.mergee.EntityType,
		"similarity":  sim,
		"sources":     sources,
	}
	if llmReason != "" {
		payload["llm_reason"] = llmReason
	}
	u.recordCurateProposal(ctx, GovernanceProposal{
		CollectionID: colID, Kind: ProposalKindEntityMerge, Risk: ProposalRiskHigh,
		Status: ProposalStatusPending, Payload: payload,
	})
	rep.ProposalsPending++
	rep.Proposals = append(rep.Proposals, CurateProposal{Kind: ProposalKindEntityMerge, Risk: ProposalRiskHigh, Status: ProposalStatusPending, DedupKey: dedup})
}

// emPayloadID 提案载荷实体 id 解析（JSON 往返后数值为 float64——
// mocParamsFromPayload 同容忍度：float64/int64/string 均受理）。
func emPayloadID(payload map[string]any, key string) (int64, bool) {
	if payload == nil {
		return 0, false
	}
	switch v := payload[key].(type) {
	case int64:
		return v, v > 0
	case int:
		return int64(v), v > 0
	case float64:
		return int64(v), v > 0
	case string:
		var n int64
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}

// applyEntityMergeDisposal 执行 entity_merge 提案 applied 的实际处置
// （ADR-D）：EntityRepo.MergeEntities 四层事务；历史提案缺字段/自对降级
// no-op（mocParamsFromPayload 先例）；keeper NotFound 视为幂等成功（orphan
// DeleteDocument NotFound 先例）；其余错误上抛（提案停留 pending 可重审）。
func (u *Usecase) applyEntityMergeDisposal(ctx context.Context, view GovernanceProposalView) error {
	keeperID, kok := emPayloadID(view.Payload, "keeper_id")
	mergeeID, mok := emPayloadID(view.Payload, "mergee_id")
	if !kok || !mok || keeperID == mergeeID {
		return nil
	}
	if u.entities == nil {
		return nil
	}
	res, err := u.entities.MergeEntities(ctx, view.CollectionID, keeperID, []int64{mergeeID})
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			u.lg.Info("治理：entity_merge 提案 applied，keeper 已不在（幂等成功）",
				loggateway.StepID("knowledge.curate.entity_merge"),
				loggateway.Int64("keeper_id", keeperID),
				loggateway.Int64("mergee_id", mergeeID),
				loggateway.Int64("proposal_id", view.ID),
			)
			return nil
		}
		return apierror.Internal(apierror.DomainKnowledge, "entity_merge disposal: merge %d → %d", mergeeID, keeperID).WithCause(err)
	}
	u.lg.Info("治理：entity_merge 提案 applied，实体已并入",
		loggateway.StepID("knowledge.curate.entity_merge"),
		loggateway.Int64("keeper_id", keeperID),
		loggateway.Int64("mergee_id", mergeeID),
		loggateway.Int("merged", res.MergedEntities),
		loggateway.Int64("proposal_id", view.ID),
	)
	return nil
}

// itoa64 十进制整形转串（dedup 键拼件）。
func itoa64(n int64) string {
	return fmt.Sprintf("%d", n)
}
