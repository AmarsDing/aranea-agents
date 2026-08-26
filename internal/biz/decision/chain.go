package decision

import (
	"context"
	"strings"
)

// 决策链 trace（M80 设计 §5，FR-80-06）：沿单父链 parent_decision_id
// 向上/向下追溯；parent 缺失时按 run 内最近前置 planner 决策补虚拟父
// （virtual_parent=true，不回写）。深度上限 20，环检测靠路径集合去重。

// maxChainDepth 是设计 §5 的深度硬上限。
const maxChainDepth = 20

// ChainDirection 是链遍历方向枚举。
type ChainDirection string

const (
	ChainUp   ChainDirection = "up"
	ChainDown ChainDirection = "down"
	ChainBoth ChainDirection = "both"
)

// normalizeChainParams 收敛 direction/max_depth：空 direction → both；
// max_depth∉[1,20] → 默认/截断为 20。
func normalizeChainParams(direction string, maxDepth int) (ChainDirection, int) {
	d := ChainDirection(strings.ToLower(strings.TrimSpace(direction)))
	switch d {
	case ChainUp, ChainDown, ChainBoth:
	default:
		d = ChainBoth
	}
	if maxDepth <= 0 || maxDepth > maxChainDepth {
		maxDepth = maxChainDepth
	}
	return d, maxDepth
}

// Chain 是一次 GetDecisionChain 的结果：Root 为锚点记录，Upstream[0]
// 是直接父（向上递远），Downstream 按深度升序（同层按时间/ID 稳定序）。
type Chain struct {
	Root       *Record
	Upstream   []Record
	Downstream []Record
}

// ChainRepo 是链追溯的读侧契约（internal/data 实现，递归 CTE）。
type ChainRepo interface {
	// ListUpstream 返回 startID 的祖先链（不含 startID 自身），[0]=直接父，
	// 最多 maxDepth 条。实现侧用递归 CTE + 深度闸。
	ListUpstream(ctx context.Context, startID int64, maxDepth int) ([]Record, error)
	// ListDownstream 返回 startID 的后代集（不含自身），深度升序，
	// 最多每层递归 maxDepth 层。
	ListDownstream(ctx context.Context, startID int64, maxDepth int) ([]Record, error)
	// FindLatestPlannerByRun 找同 run 内 created_at <= before 的最近
	// planner_orchestration 决策（excludeID 排除自身）；未命中返回 nil, nil。
	FindLatestPlannerByRun(ctx context.Context, runID, beforeCreatedAt string, excludeID int64) (*Record, error)
}

// GetChain 追溯 decisionKey 的决策链。root 未命中返回 nil, nil。
// repo 为 nil（无库/CLI）时返回 nil, nil。
func (u *QueryUsecase) GetChain(ctx context.Context, decisionKey, direction string, maxDepth int) (*Chain, error) {
	if u == nil || u.repo == nil || strings.TrimSpace(decisionKey) == "" {
		return nil, nil
	}
	crepo, ok := u.repo.(ChainRepo)
	if !ok {
		return nil, nil
	}
	root, err := u.repo.GetByKey(ctx, decisionKey)
	if err != nil || root == nil {
		return nil, err
	}
	dir, depth := normalizeChainParams(direction, maxDepth)
	chain := &Chain{Root: root}

	if dir == ChainUp || dir == ChainBoth {
		up, err := u.chainUpstream(ctx, crepo, root, depth)
		if err != nil {
			return nil, err
		}
		chain.Upstream = up
	}
	if dir == ChainDown || dir == ChainBoth {
		down, err := crepo.ListDownstream(ctx, root.ID, depth)
		if err != nil {
			return nil, err
		}
		chain.Downstream = dedupChainRecords(down, root.ID)
	}
	return chain, nil
}

// chainUpstream 上游：有真实父链走 CTE 追溯；无父但带 run_id 时按设计 §5
// 兜底补同 run 内最近前置 planner 决策为虚拟父（仅单节点，不回写）。
func (u *QueryUsecase) chainUpstream(ctx context.Context, crepo ChainRepo, root *Record, depth int) ([]Record, error) {
	if root.ParentDecisionID != nil {
		up, err := crepo.ListUpstream(ctx, root.ID, depth)
		if err != nil {
			return nil, err
		}
		return dedupChainRecords(up, root.ID), nil
	}
	if root.SourceRef.RunID == "" {
		return nil, nil
	}
	vp, err := crepo.FindLatestPlannerByRun(ctx, root.SourceRef.RunID, root.CreatedAt, root.ID)
	if err != nil || vp == nil {
		return nil, err
	}
	vp.VirtualParent = true
	return []Record{*vp}, nil
}

// dedupChainRecords 环检测：路径集合去重（防御脏数据自环），并剔除
// 锚点自身（脏数据 id=parent_decision_id 时 CTE 会带回自身）。
func dedupChainRecords(recs []Record, rootID int64) []Record {
	if len(recs) == 0 {
		return nil
	}
	seen := make(map[int64]bool, len(recs))
	out := make([]Record, 0, len(recs))
	for _, r := range recs {
		if r.ID == rootID || seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		out = append(out, r)
	}
	return out
}
