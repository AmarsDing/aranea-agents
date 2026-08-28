package agent

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/biz"
)

// intentKindDomains maps intent-pass intent_kind onto a lexicon domain so
// composite slots can bind the roster without an extra LLM hop.
var intentKindDomains = map[string]string{
	"research":    "研究/调研",
	"creative":    "创作",
	"analysis":    "数据/分析",
	"code_change": "软件",
	"debug":       "软件",
	"doc":         "办公/文档",
	"explain":     "办公/文档",
}

const subIntentNameMaxRunes = 40

// subTasksFromSubIntents turns a composite intent artifact into ordered
// subtasks. Returns nil unless at least two non-empty goals are present.
// Later slots depend on the previous one so "查数据再发邮件" stays sequential.
func subTasksFromSubIntents(art *biz.IntentArtifact) []biz.SubTask {
	if art == nil || len(art.SubIntents) < 2 {
		return nil
	}
	out := make([]biz.SubTask, 0, len(art.SubIntents))
	prevID := ""
	for i, s := range art.SubIntents {
		goal := strings.TrimSpace(s.Goal)
		if goal == "" {
			continue
		}
		id := fmt.Sprintf("st_intent_%d", i+1)
		st := biz.SubTask{
			ID:                   id,
			Name:                 truncateRunes(goal, subIntentNameMaxRunes),
			Description:          goal,
			Priority:             i + 1,
			DomainPath:           domainPathFromSubIntent(s),
			RequiredCapabilities: append([]string(nil), s.ToolHints...),
		}
		if prevID != "" {
			st.DependsOn = []string{prevID}
		}
		out = append(out, st)
		prevID = id
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

func domainPathFromSubIntent(s biz.SubIntent) string {
	if spec, ok := intentKindDomains[strings.ToLower(strings.TrimSpace(s.IntentKind))]; ok {
		return spec
	}
	if d := inferGenericRoleDomain("", s.Goal+" "+s.IntentKind); d != "" {
		return d
	}
	return ""
}

func truncateRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}
