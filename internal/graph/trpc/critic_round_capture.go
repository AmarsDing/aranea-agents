package graph

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
)

// criticLoopFinishAgentNodeIDs returns agent-node IDs that are the source of a
// critic_loop conditional edge — i.e. the critic (finish) node of a review
// loop. Only agent nodes need the round-capture callback: their output lands
// in StateKeyLastResponse / StateKeyNodeResponses (never StateKeyMessages),
// so the cond func could otherwise not count evaluation rounds.
func criticLoopFinishAgentNodeIDs(cfg GraphBuildConfig) map[string]struct{} {
	fromIDs := map[string]struct{}{}
	for _, ce := range cfg.ConditionalEdges {
		if _, _, _, ok := biz.ParseCriticLoopCondFuncRef(strings.TrimSpace(ce.CondFuncRef)); !ok {
			continue
		}
		if from := strings.TrimSpace(ce.From); from != "" {
			fromIDs[from] = struct{}{}
		}
	}
	if len(fromIDs) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(fromIDs))
	for _, n := range cfg.Nodes {
		if _, ok := fromIDs[n.ID]; !ok {
			continue
		}
		if normalizeNodeType(n.Type) == biz.NodeTypeAgent {
			out[n.ID] = struct{}{}
		}
	}
	return out
}

// criticRoundCaptureCallbackForNode returns an AfterNodeCallback that records
// one completed critic evaluation round into node-scoped state metadata:
// rounds counter + the last two critic responses (for loop-until-dry
// detection). It runs on critic_loop finish (agent) nodes and rewrites the
// node result delta to carry the updated metadata map. Team graphs do not
// register a metadata schema field, so the whole map is cloned and written
// back (overwrite semantics) — no other keys are clobbered.
//
// Keys are scoped by nodeID (biz.CriticLoopMetaKeysForNode) so multiple critic
// loops in one graph converge independently. Read base falls back to the
// legacy bare keys once (checkpoints written before scoping was introduced);
// writes always go to the scoped keys. Fail-open: any unexpected shape leaves
// the result untouched.
func criticRoundCaptureCallbackForNode(nodeID string) trpcgraph.AfterNodeCallback {
	roundsKey, lastKey, prevKey := biz.CriticLoopMetaKeysForNode(nodeID)
	return func(
		ctx context.Context,
		callbackCtx *trpcgraph.NodeCallbackContext,
		state trpcgraph.State,
		result any,
		nodeErr error,
	) (any, error) {
		if nodeErr != nil {
			return nil, nil
		}
		upd, ok := result.(trpcgraph.State)
		if !ok || upd == nil {
			return nil, nil
		}
		curr, _ := trpcgraph.GetStateValue[string](upd, trpcgraph.StateKeyLastResponse)
		if strings.TrimSpace(curr) == "" {
			return nil, nil
		}
		meta := map[string]any{}
		if existing, ok := state[trpcgraph.StateKeyMetadata].(map[string]any); ok {
			for k, v := range existing {
				meta[k] = v
			}
		}
		rounds := biz.CriticLoopMetaInt(meta[roundsKey])
		prev, _ := meta[lastKey].(string)
		if rounds == 0 && prev == "" && nodeID != "" {
			// 旧 checkpoint 回落：升级前写入的裸 key 只读取一次作为基数，
			// 本轮起写入 scoped key，之后不再触碰裸 key。
			rounds = biz.CriticLoopMetaInt(meta[biz.CriticLoopRoundsMetaKey])
			prev, _ = meta[biz.CriticLoopLastResponseMetaKey].(string)
		}
		meta[prevKey] = prev
		meta[lastKey] = curr
		meta[roundsKey] = rounds + 1
		cloned := upd.Clone()
		cloned[trpcgraph.StateKeyMetadata] = meta
		return cloned, nil
	}
}
