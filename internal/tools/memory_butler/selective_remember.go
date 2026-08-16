package memory_butler

import (
	"context"
	"math"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/skill"
	"aranea-agents/pkg/jsonutil"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type selectiveRememberInput struct {
	Content string `json:"content" jsonschema:"description=要记忆的内容,required"`
	Context string `json:"context" jsonschema:"description=记忆的上下文信息"`
	AgentID string `json:"agent_id" jsonschema:"description=Agent ID,required"`
}

const (
	// selectiveRememberConfidence/Importance：butler 精选事实的默认值。
	// 此前零值写入 → 召回侧阈值（l4CueMinConfidence=0.3、episode 提取
	// minImportance=0.3）永久过滤，butler 记的事实永远召不回。
	selectiveRememberConfidence = 0.8
	selectiveRememberImportance = 0.7
	// selectiveRememberSemanticDupScore 语义去重余弦阈值，对齐写管线合并
	// 阈值 FactWriteMergeScore（0.92，同 conflict supersede 判据）。
	selectiveRememberSemanticDupScore = 0.92
)

type selectiveRememberOutput struct {
	Remembered bool   `json:"remembered"`
	Reason     string `json:"reason"`
}

func newSelectiveRememberTool(deps Deps) trpctool.Tool {
	execute := func(ctx context.Context, input selectiveRememberInput) (selectiveRememberOutput, error) {
		if input.AgentID == "" {
			return selectiveRememberOutput{}, ErrAgentIDRequired
		}
		if input.Content == "" {
			return selectiveRememberOutput{}, ErrContentRequired
		}

		// Check for redundancy by listing existing facts and comparing.
		rows, _, _, _, err := deps.MemoryAdmin.ListFactRows(ctx, biz.ListFactRowsParams{
			ScopeType: "agent",
			ScopeID:   input.AgentID,
			Limit:     defaultFactListLimit,
			Offset:    0,
		})
		if err != nil {
			return selectiveRememberOutput{}, err
		}

		contentLower := strings.ToLower(strings.TrimSpace(input.Content))
		var statements []string
		for _, raw := range rows {
			m, _ := jsonutil.ParseMap(raw)
			if m == nil {
				continue
			}
			existing := strings.ToLower(strings.TrimSpace(jsonutil.IfaceStr(m, "statement")))
			if existing == "" {
				continue
			}
			// 第一遍：廉价字符串判重（精确/子串）。
			if contentLower == existing {
				return selectiveRememberOutput{Remembered: false, Reason: "redundant with existing memory"}, nil
			}
			if len(contentLower) > minLengthForSubstringCheck && len(existing) > minLengthForSubstringCheck {
				if strings.Contains(contentLower, existing) || strings.Contains(existing, contentLower) {
					return selectiveRememberOutput{Remembered: false, Reason: "redundant with existing memory"}, nil
				}
			}
			statements = append(statements, jsonutil.IfaceStr(m, "statement"))
		}

		// 第二遍：语义判重（P2-3）。Embedder 已接线（MultiProviderEmbedder）；
		// nil 或 embed 失败降级为仅字符串判重，不阻断写入。
		if deps.Embedder != nil && len(statements) > 0 {
			if semDup, serr := semanticDuplicate(ctx, deps.Embedder, input.Content, statements); serr != nil {
				deps.LG.Warn("selective_remember 语义判重失败，降级字符串判重", loggateway.Err(serr))
			} else if semDup {
				return selectiveRememberOutput{Remembered: false, Reason: "semantically redundant with existing memory"}, nil
			}
		}

		// Novel content — write as a new fact.
		_, err = deps.MemoryAdmin.UpsertFactRow(ctx, biz.FactUpsert{
			Statement:  input.Content,
			ScopeType:  "agent",
			ScopeID:    input.AgentID,
			AgentID:    input.AgentID,
			FactKind:   "semantic",
			Confidence: selectiveRememberConfidence,
			Importance: selectiveRememberImportance,
			SourceKind: "selective_remember",
			Status:     "active",
		})
		if err != nil {
			return selectiveRememberOutput{}, err
		}
		return selectiveRememberOutput{Remembered: true, Reason: "novel content worth remembering"}, nil
	}
	return function.NewFunctionTool(
		execute,
		function.WithName("memory_butler_selective_remember"),
		function.WithDescription("选择性记忆：判断内容是否值得记忆，避免冗余存储。若内容与已有记忆重复则跳过，否则写入新记忆。"),
	)
}

// semanticDuplicate 用 Embedder 对 content 与既有 statements 做批量向量判重：
// 任一余弦 ≥ selectiveRememberSemanticDupScore 视为语义冗余。
func semanticDuplicate(ctx context.Context, embedder skill.SkillEmbedder, content string, statements []string) (bool, error) {
	texts := make([]string, 0, len(statements)+1)
	texts = append(texts, strings.TrimSpace(content))
	texts = append(texts, statements...)
	vecs, err := embedder.Embed(ctx, texts)
	if err != nil {
		return false, err
	}
	if len(vecs) != len(texts) || len(vecs[0]) == 0 {
		return false, nil
	}
	for _, v := range vecs[1:] {
		if cosineSimilarity32(vecs[0], v) >= selectiveRememberSemanticDupScore {
			return true, nil
		}
	}
	return false, nil
}

// cosineSimilarity32 计算两个 float32 向量的余弦相似度（长度不等/零向量返回 0）。
func cosineSimilarity32(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
