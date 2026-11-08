// latency 物化器:把 latency(A.side, B.side) 类指标从"算不出"变成真实值。
// 口径:
//   - 声明形如 expr: "latency(b2.enter, b3.enter)" + join: "payload.signal_id";
//     A/B 解析为同工作流内节点引用(b2 = 节点 id 前缀 b2 的节点;.enter/.exit 取对应 subject);
//   - 配对键:join 指定的 payload 点路径 → 事件 entity_id(归一化时已按 field_map.entity_id 提取;
//     若 join 与节点 entity_id 路径不同,以 join 为准的提取需事件侧带该字段——本版要求 join ==
//     节点 entity_id 路径,否则物化器跳过并记原因,不编数据);
//   - 配对规则:同 entity_id,B.At > A.At,取该 B 之前最近的 A;每对产出一条样本(B 侧幂等);
//   - 时钟口径:occurred_at 缺失时退 received_at(已在 LatencyStore.ListPairEvents 归一);
//   - 指标求值:rolling_p50 → 窗口内样本 p50;rolling_p50_p95 → p50/p95(展示 p50,note 带 p95)。
//
// 移植说明(整合 P2-4):存储依赖收敛为 LatencyStore 窄接口(P2-5 Ent 仓储实现);
// 平台映射由构造参数注入;日志走 loggateway.Logger(禁 log/slog)。
package agg

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"time"

	"aranea-agents/internal/biz/ceoboard/adapter"
	"aranea-agents/internal/biz/ceoboard/registry"
	"aranea-agents/internal/biz/ceoboard/store"
	"aranea-agents/pkg/loggateway"
)

// LatencyStore 是物化器对事件存储的依赖面(P2-5 由 Ent 仓储实现)。
type LatencyStore interface {
	// ListPairEvents 取配对候选(entity_id 非空,received_at >= since,升序,上限由实现封顶)。
	ListPairEvents(platform, subjectKey string, since time.Time) ([]store.PairEvent, error)
	// UpsertLatencySample 幂等写入(同 metric_id+b_uid 重复忽略);返回 true 表示新插入。
	UpsertLatencySample(sm *store.LatencySample) (bool, error)
}

// LatencyMaterializer 周期把事件表配对成延迟样本。
type LatencyMaterializer struct {
	reg *registry.Registry
	st  LatencyStore
	// natsPlatforms: endpoint host:port → 平台 id(与原 config.NATSPlatforms 同义)。
	natsPlatforms map[string]string
	log           loggateway.Logger // 可空:空时静默(单测)
}

// NewLatencyMaterializer 构造。log 可传 nil(单测静默)。
func NewLatencyMaterializer(reg *registry.Registry, st LatencyStore, natsPlatforms map[string]string, log loggateway.Logger) *LatencyMaterializer {
	return &LatencyMaterializer{reg: reg, st: st, natsPlatforms: natsPlatforms, log: log}
}

// latencyRef 是 latency(A.enter) 的一侧引用。
type latencyRef struct {
	nodeIDPrefix string // 节点 id 前缀(b2 → b2-signal)
	side         string // enter | exit
	node         *registry.Node
	subjectKey   string
	platform     string
}

var latencyExprRe = regexp.MustCompile(`^latency\(\s*([a-z0-9_-]+)\.(enter|exit)\s*,\s*([a-z0-9_-]+)\.(enter|exit)\s*\)$`)

// ParseLatencyExpr 解析 latency(a.enter, b.exit) → (A引用, B引用);非该形返回 nil。
func ParseLatencyExpr(expr string) []latencyRef {
	m := latencyExprRe.FindStringSubmatch(expr)
	if len(m) != 5 {
		return nil
	}
	return []latencyRef{
		{nodeIDPrefix: m[1], side: m[2]},
		{nodeIDPrefix: m[3], side: m[4]},
	}
}

// resolveRef 把节点 id 前缀解析为节点 + subject_key + 平台。
func (m *LatencyMaterializer) resolveRef(wf *registry.Workflow, ref *latencyRef) error {
	for i := range wf.Nodes {
		n := &wf.Nodes[i]
		if len(n.ID) >= len(ref.nodeIDPrefix) && n.ID[:len(ref.nodeIDPrefix)] == ref.nodeIDPrefix {
			cond := n.Enter
			if ref.side == "exit" {
				cond = n.Exit
			}
			subj, ok := subjectOfCond(cond)
			if !ok {
				return fmt.Errorf("节点 %s 的 %s 非 event(subject=...) 形", n.ID, ref.side)
			}
			ref.node = n
			ref.subjectKey = subj
			if n.Source != nil {
				ref.platform = m.natsPlatforms[adapter.HostPort(n.Source.Endpoint)]
			}
			if ref.platform == "" {
				return fmt.Errorf("节点 %s 无 nats endpoint 或平台映射缺失", n.ID)
			}
			return nil
		}
	}
	return fmt.Errorf("找不到节点前缀 %s", ref.nodeIDPrefix)
}

// MaterializeOnce 跑一轮配对(幂等;可手工调用做验证)。
// 返回 (产出样本数, 跳过原因列表)。
func (m *LatencyMaterializer) MaterializeOnce(now time.Time) (int, []string) {
	produced := 0
	var notes []string
	if m.st == nil {
		return 0, []string{"store 未接线"}
	}
	for _, wf := range m.reg.List() {
		for ni := range wf.Nodes {
			n := &wf.Nodes[ni]
			for mi := range n.Metrics {
				mt := &n.Metrics[mi]
				refs := ParseLatencyExpr(mt.Expr)
				if refs == nil {
					continue
				}
				if err := m.resolveRef(wf, &refs[0]); err != nil {
					notes = append(notes, fmt.Sprintf("%s: A 侧 %v", mt.ID, err))
					continue
				}
				if err := m.resolveRef(wf, &refs[1]); err != nil {
					notes = append(notes, fmt.Sprintf("%s: B 侧 %v", mt.ID, err))
					continue
				}
				// join 一致性:配对语义建立在 entity_id 列上;节点提取路径须与 join 相同(或缺省),否则跳过,不编配对
				join := mt.Join
				mismatch := false
				for _, r := range refs {
					em := ""
					if r.node.Source != nil && r.node.Source.FieldMap != nil {
						em = r.node.Source.FieldMap["entity_id"]
					}
					if join != "" && em != "" && em != join {
						notes = append(notes, fmt.Sprintf("%s: 节点 %s entity_id 路径 %q ≠ join %q,跳过", mt.ID, r.node.ID, em, join))
						mismatch = true
					}
				}
				if mismatch {
					continue
				}
				n2, err := m.pairMetric(mt.ID, refs[0], refs[1], now)
				if err != nil {
					notes = append(notes, fmt.Sprintf("%s: %v", mt.ID, err))
					continue
				}
				produced += n2
			}
		}
	}
	return produced, notes
}

// pairMetric 配对单个指标:A/B 两侧各取窗口候选,同 entity 配对,B 侧幂等落样本。
func (m *LatencyMaterializer) pairMetric(metricID string, a, b latencyRef, now time.Time) (int, error) {
	// 配对搜索窗:覆盖长尾(24h);样本按 B.At 落窗,查询侧再按指标 window 过滤
	since := now.Add(-24 * time.Hour)
	as, err := m.st.ListPairEvents(a.platform, a.subjectKey, since)
	if err != nil {
		return 0, fmt.Errorf("A 侧取数: %w", err)
	}
	bs, err := m.st.ListPairEvents(b.platform, b.subjectKey, since)
	if err != nil {
		return 0, fmt.Errorf("B 侧取数: %w", err)
	}
	if len(as) == 0 || len(bs) == 0 {
		return 0, nil
	}
	// entity → A 事件升序序列
	aByEntity := map[string][]store.PairEvent{}
	for _, e := range as {
		aByEntity[e.EntityID] = append(aByEntity[e.EntityID], e)
	}
	produced := 0
	for _, bev := range bs {
		cands := aByEntity[bev.EntityID]
		// 取 B 之前最近的 A
		var best *store.PairEvent
		for i := range cands {
			if cands[i].At.Before(bev.At) || cands[i].At.Equal(bev.At) {
				cp := cands[i]
				best = &cp
			}
		}
		if best == nil {
			continue
		}
		lat := bev.At.Sub(best.At).Milliseconds()
		if lat < 0 {
			continue
		}
		new, err := m.st.UpsertLatencySample(&store.LatencySample{
			MetricID: metricID, BUID: bev.UID, AUID: best.UID,
			EntityID: bev.EntityID, LatencyMs: lat, At: bev.At,
		})
		if err != nil {
			return produced, fmt.Errorf("样本落库: %w", err)
		}
		if new {
			produced++
		}
	}
	return produced, nil
}

// RunLoop 周期物化直到 ctx 取消。
func (m *LatencyMaterializer) RunLoop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, notes := m.MaterializeOnce(time.Now())
			if m.log == nil {
				continue
			}
			for _, note := range notes {
				m.log.Warn("[latency] 跳过", loggateway.Str("reason", note))
			}
			if n > 0 {
				m.log.Info("[latency] 本轮产出延迟样本", loggateway.Int("produced", n))
			}
		}
	}
}

// Percentile 计算样本的 p 分位(0-100);空样本返回 0,false。
func Percentile(samples []int64, p float64) (float64, bool) {
	if len(samples) == 0 {
		return 0, false
	}
	s := append([]int64{}, samples...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	idx := int(p / 100 * float64(len(s)-1))
	return float64(s[idx]), true
}
