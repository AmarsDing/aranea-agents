package adapter

import (
	"context"
	"strings"
	"unicode"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const DefaultCriticLoopThreshold = 0.0

// criticLoopCondFunc builds the critic_loop conditional edge function.
// maxIterations > 0 且达到上限仍未批准时强制收敛，返回
// biz.CriticLoopResultApprovedForced（与真实批准的 "approved" 区分，便于
// 观测「上限兜底收敛」）；编译期 PathMap 须将该键映射到 EndNodeID。
//
// nodeID 为该条件边 From 的 critic 节点 ID（经 CondFuncRef "%<nodeID>" 段
// 编码传入），轮次/反馈按节点隔离读取（biz.CriticLoopMetaKeysForNode），
// 多 critic 图各自独立收敛；空 nodeID（裸 ref，图域 LLM 节点路径）读裸 key。
//
// 判定顺序（显式信号优先于启发式）：
//  1. 结构化裁决（末条消息的 orchestration_control 工具调用）— 最高优先级。
//     批准 → approved；未批准 → 达上限 approved_forced，否则 retry。
//     显式 retry 不被 dry 收敛/关键词/打分推翻（F3）。
//  2. 评审文本中文/英文批准词（last_response 优先于 messages 末条）。
//  3. 评分 >= threshold（threshold > 0 时）。
//  4. 达到迭代上限 → approved_forced（上限前一轮的批准仍算真实批准）。
//  5. loop-until-dry：最近两轮反馈归一化后相同，提前收敛 approved。
//
// 轮次与反馈来源（按优先级）：
//  1. state metadata（节点 scoped 的 critic_loop_rounds/*_response，回落裸 key）—
//     由 graph/trpc 的 critic-round capture callback 写入，覆盖 team 图的
//     agent 节点 critic（agent 节点输出只进 last_response / node_responses，
//     不进 messages）。
//  2. messages 中的 orchestration_control 工具调用 — LLM 节点 critic 的结构化路径。
func criticLoopCondFunc(threshold float64, maxIterations int, nodeID string, lg loggateway.Logger) trpcgraph.ConditionalFunc {
	return func(ctx context.Context, state trpcgraph.State) (string, error) {
		msgs, _ := state[trpcgraph.StateKeyMessages].([]trpcmodel.Message)
		criticMsgs := collectCriticMessages(msgs)
		rounds, prevFeedback, lastFeedback := criticRoundState(state, nodeID)
		if rounds < len(criticMsgs) {
			rounds = len(criticMsgs)
		}
		forced := maxIterations > 0 && rounds >= maxIterations
		// 1. 结构化裁决。
		if len(msgs) > 0 {
			lastMsg := msgs[len(msgs)-1]
			for _, tc := range lastMsg.ToolCalls {
				if tc.Function.Name != biz.OrchestrationControlToolName {
					continue
				}
				d, err := biz.ParseOrchestrationDecision(tc.Function.Arguments, lg)
				if err != nil {
					continue
				}
				if biz.IsApprovedDecision(d, threshold) {
					return "approved", nil
				}
				if forced {
					lg.Info("critic_loop 达到迭代上限，强制收敛",
						loggateway.Int("rounds", rounds),
						loggateway.Int("max_iterations", maxIterations))
					return biz.CriticLoopResultApprovedForced, nil
				}
				return "retry", nil
			}
		}
		// 2/3. 评审文本批准词与评分兜底。
		content := criticReviewContent(state, msgs)
		if isCriticApprovalContent(content) {
			return "approved", nil
		}
		if threshold > 0 {
			if score := biz.ExtractScore(strings.ToLower(content)); score > 0 && score >= threshold {
				return "approved", nil
			}
		}
		// 4. 上限兜底：仍无批准信号才记强制收敛。
		if forced {
			lg.Info("critic_loop 达到迭代上限，强制收敛",
				loggateway.Int("rounds", rounds),
				loggateway.Int("max_iterations", maxIterations))
			return biz.CriticLoopResultApprovedForced, nil
		}
		// 5. loop-until-dry：最近两轮 critic 反馈无实质变化，继续迭代无收益，提前收敛。
		if prevFeedback != "" && normalizeCriticContent(prevFeedback) == normalizeCriticContent(lastFeedback) {
			lg.Info("critic_loop 反馈无新意见，提前收敛", loggateway.Int("rounds", rounds))
			return "approved", nil
		}
		if n := len(criticMsgs); n >= 2 && criticFeedbackDry(criticMsgs[n-2], criticMsgs[n-1]) {
			lg.Info("critic_loop 反馈无新意见，提前收敛", loggateway.Int("rounds", n))
			return "approved", nil
		}
		return "retry", nil
	}
}

// criticRoundState reads the capture-callback recorded critic rounds and the
// last two critic feedback texts from state metadata, scoped by critic nodeID
// (biz.CriticLoopMetaKeysForNode). Falls back to the legacy bare keys when the
// scoped keys carry no data yet (checkpoints written before scoping). All
// empty when the callback is not wired (e.g. LLM-node critics, which use the
// messages path).
func criticRoundState(state trpcgraph.State, nodeID string) (rounds int, prev, last string) {
	meta, ok := state[trpcgraph.StateKeyMetadata].(map[string]any)
	if !ok {
		return 0, "", ""
	}
	roundsKey, lastKey, prevKey := biz.CriticLoopMetaKeysForNode(nodeID)
	rounds = biz.CriticLoopMetaInt(meta[roundsKey])
	prev, _ = meta[prevKey].(string)
	last, _ = meta[lastKey].(string)
	if rounds == 0 && prev == "" && last == "" && nodeID != "" {
		rounds = biz.CriticLoopMetaInt(meta[biz.CriticLoopRoundsMetaKey])
		prev, _ = meta[biz.CriticLoopPrevResponseMetaKey].(string)
		last, _ = meta[biz.CriticLoopLastResponseMetaKey].(string)
	}
	return rounds, prev, last
}

// criticReviewContent returns the critic's latest review text: the graph
// last_response (agent-node path) when set, else the last message content.
func criticReviewContent(state trpcgraph.State, msgs []trpcmodel.Message) string {
	if s, ok := state[trpcgraph.StateKeyLastResponse].(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	if len(msgs) > 0 {
		return msgs[len(msgs)-1].Content
	}
	return ""
}

// 中文审批词汇。拒绝词必须先于批准词判断（“不批准”含“批准”）。
// 不使用裸“通过”——中文里常作介词（“通过描绘…”），误报率高。
var criticApprovalPhrasesZH = []string{
	"批准", "评审通过", "通过评审", "审核通过", "予以通过", "同意发布", "无异议",
	"结论：通过", "结论:通过",
}

var criticRejectionPhrasesZH = []string{
	"不批准", "未通过", "不通过", "不予通过", "不同意", "驳回",
}

// 中文否定标记：紧邻批准词之前出现时构成组合式否定（如「不能予以通过」
// 「不予评审通过」），拒绝词表无法枚举全部组合。单字标记已覆盖以其结尾
// 的组合（绝不/暂不→不、并非/绝非→非），组合标记仅列不以单字结尾者。
var criticNegationMarkersZH = []string{
	"不", "未", "非", "勿", "莫",
	"不予", "未予", "不能", "未能", "不可", "不应", "无法", "难以",
}

// isCriticApprovalContent reports whether the critic review text carries an
// approval verdict (English "approved" without negation, or a Chinese approval
// phrase without a rejection phrase or adjacent negation prefix).
func isCriticApprovalContent(content string) bool {
	if content == "" {
		return false
	}
	for _, rej := range criticRejectionPhrasesZH {
		if strings.Contains(content, rej) {
			return false
		}
	}
	for _, ap := range criticApprovalPhrasesZH {
		if containsNonNegatedPhrase(content, ap) {
			return true
		}
	}
	lower := strings.ToLower(content)
	return containsWord(lower, "approved") && !containsNegationBeforeWord(lower, "approved")
}

// containsNonNegatedPhrase reports whether phrase occurs in content with at
// least one occurrence NOT directly preceded by a Chinese negation marker.
// 逐个出现位置判定：「不予评审通过；修改后评审通过」第二次命中无前缀否定，
// 仍算批准。
func containsNonNegatedPhrase(content, phrase string) bool {
	for {
		idx := strings.Index(content, phrase)
		if idx < 0 {
			return false
		}
		if !hasCriticNegationPrefix(content[:idx]) {
			return true
		}
		content = content[idx+len(phrase):]
	}
}

// hasCriticNegationPrefix reports whether the text immediately preceding an
// approval-phrase match ends with a Chinese negation marker. 仅看紧邻词尾，
// 正文别处的「不」（如「问题不复存在，予以通过」）不会造成误判。
func hasCriticNegationPrefix(prefix string) bool {
	tail := strings.TrimSpace(prefix)
	for _, neg := range criticNegationMarkersZH {
		if strings.HasSuffix(tail, neg) {
			return true
		}
	}
	return false
}

// collectCriticMessages returns messages carrying an orchestration_control
// tool call — one per critic evaluation round.
func collectCriticMessages(msgs []trpcmodel.Message) []trpcmodel.Message {
	var out []trpcmodel.Message
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			if tc.Function.Name == biz.OrchestrationControlToolName {
				out = append(out, m)
				break
			}
		}
	}
	return out
}

// criticFeedbackDry reports whether two consecutive critic rounds carry no new
// substantive feedback (normalized content identical and non-empty).
func criticFeedbackDry(prev, curr trpcmodel.Message) bool {
	p := normalizeCriticContent(prev.Content)
	return p != "" && p == normalizeCriticContent(curr.Content)
}

func normalizeCriticContent(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

func containsWord(s, word string) bool {
	for {
		idx := strings.Index(s, word)
		if idx < 0 {
			return false
		}
		beforeOk := idx == 0 || !isAlphaNum(rune(s[idx-1]))
		afterIdx := idx + len(word)
		afterOk := afterIdx >= len(s) || !isAlphaNum(rune(s[afterIdx]))
		if beforeOk && afterOk {
			return true
		}
		s = s[afterIdx:]
	}
}

func isAlphaNum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func containsNegationBeforeWord(s, word string) bool {
	negations := []string{"not", "no", "never", "don't", "doesn't", "isn't", "wasn't", "won't", "can't", "couldn't", "shouldn't", "wouldn't"}
	for {
		idx := strings.Index(s, word)
		if idx < 0 {
			return false
		}
		beforeOk := idx == 0 || !isAlphaNum(rune(s[idx-1]))
		afterIdx := idx + len(word)
		afterOk := afterIdx >= len(s) || !isAlphaNum(rune(s[afterIdx]))
		if beforeOk && afterOk {
			prefix := strings.TrimSpace(s[:idx])
			words := strings.Fields(prefix)
			start := len(words) - 3
			if start < 0 {
				start = 0
			}
			for _, w := range words[start:] {
				for _, neg := range negations {
					if w == neg {
						return true
					}
				}
			}
			return false
		}
		s = s[afterIdx:]
	}
}

func RegisterCriticLoopCondFunc(reg RegistryRegistrar, threshold float64, lg loggateway.Logger) {
	fn := criticLoopCondFunc(threshold, 0, "", lg)
	reg.RegisterCondFuncInstance(biz.CriticLoopCondFuncRef, fn)
	if threshold > 0 {
		reg.RegisterCondFuncInstance(biz.CriticLoopCondFuncRefForThreshold(threshold), fn)
	}
}

// EnsureCriticLoopCondFuncs registers parameterized critic_loop CondFuncRefs
// ("critic_loop[@<threshold>][#<maxIterations>][%<nodeID>]" 及其子集) found in
// cfg so ResolveBuildConfig succeeds for per-team score_threshold /
// max_iterations，且 nodeID 经 ref 传入 cond func 实现按节点隔离的轮次统计。
// 裸 ref（无参数）由 RegisterCriticLoopCondFunc 注册，此处跳过。
func EnsureCriticLoopCondFuncs(reg RegistryRegistrar, cfg biz.GraphBuildConfig, lg loggateway.Logger) {
	if reg == nil {
		return
	}
	for _, ce := range cfg.ConditionalEdges {
		ref := strings.TrimSpace(ce.CondFuncRef)
		threshold, maxIter, nodeID, ok := biz.ParseCriticLoopCondFuncRef(ref)
		if !ok || (threshold <= 0 && maxIter <= 0 && nodeID == "") {
			continue
		}
		reg.RegisterCondFuncInstance(ref, criticLoopCondFunc(threshold, maxIter, nodeID, lg))
	}
}

type RegistryRegistrar interface {
	RegisterCondFuncInstance(name string, fn any)
}
