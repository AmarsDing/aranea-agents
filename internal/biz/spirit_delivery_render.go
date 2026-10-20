package biz

import (
	"encoding/json"
	"fmt"
	"reflect"

	"sort"

	"strings"
	"time"
	"unicode/utf8"
)

// renderPayloadContent renders one payload value as standalone full text:
// document-paradigm maps prepend 《title》 to their content, plain strings
// pass through, other shapes serialize as indented JSON.

func renderPayloadContent(v any) string {
	switch payload := v.(type) {
	case map[string]any:
		content, _ :=
			payload["content"].(string)
		title, _ := payload["title"].(string)
		content,
			title = strings.TrimSpace(content), strings.TrimSpace(title)
		if content !=
			"" {
			if title != "" {
				return "《" + title + "》\n\n" +
					content
			}
			return content
		}
	case string:
		return strings.TrimSpace(payload)
	}
	b,
		err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}
func extractKeyFindings(content string) string {
	var findings []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		isBullet := strings.HasPrefix(trimmed,
			"- ") || strings.HasPrefix(trimmed, "* ")
		isNumbered := isNumberedListItem(trimmed)
		isQuote := strings.HasPrefix(trimmed, "> ")
		if (isBullet ||
			isNumbered) && !isQuote && len(findings) < MaxKeyFindingsCount {
			findings = append(findings, trimmed)
		}
	}
	return strings.Join(findings, "\n")
}
func isNumberedListItem(s string) bool {
	if len(s) <
		3 {
		return false
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i ==
		0 || i >= len(s) {
		return false
	}
	return (s[i] == '.' || s[i] == ')') && i+1 < len(s) &&
		s[i+1] ==
			' '
} // memberEvidenceSummaryMaxRunes caps the failed-step summary carried in a

// MemberExecutionEvidence reason (log + projection diagnostics only).

const memberEvidenceSummaryMaxRunes = 120 // memberStepWindow aggregates the execution window across steps: start is the

// earliest non-zero StartedAt; end is the latest activity evidence, preferring

// CompletedAt over StartedAt per step. authorFilter non-empty restricts to

// steps authored by that agent key. ok=false means no step carried a non-zero

// StartedAt.
//
// notice steps are excluded (2026-08-08 回归发现): context_usage 等被动通知以

// 成员 author 持续落到 run 结束，会把 end 撑大到远超成员真实工作完成时刻

// （真实 reply 04:54:42，notice 拖到 04:59:15），且 publish 查询与 notice 落库

// 存在时序 race，窗口结果不稳定。notice 不产生任何执行工作量，不是活动证据。
func memberStepWindow(steps []Step, authorFilter string) (start, end time.Time,
	ok bool) {
	for _, st := range steps {
		if authorFilter != "" &&
			strings.TrimSpace(st.AuthorAgentKey) !=
				authorFilter {
			continue

		}
		if st.Kind == StepKindNotice {
			continue
		}
		if !st.StartedAt.IsZero() {
			if start.
				IsZero() || st.StartedAt.Before(start) {
				start = st.StartedAt
			}
		}
		candidate := st.StartedAt
		if st.CompletedAt !=
			nil && !st.CompletedAt.IsZero() {
			candidate = *st.CompletedAt

		}
		if !candidate.IsZero() && (end.
			IsZero() || candidate.After(end)) {
			end =
				candidate
		}
	}
	return start, end, !start.IsZero()
} // Reserved keys of the graph deliverable state map. Both are extracted into

// first-class DeliverableRef fields by WriteDeliverablesToSession and are

// therefore excluded from StructuredJSON.
const (
	deliverableReservedKeySummary   = "summary"
	deliverableReservedKeyCognition = "cognition"
) // extractStateCognition converts the reserved "cognition" state-map entry

// into a DeliverableCognition. The value arrives as a map[string]any (JSON
// round-trip from graph state), so it is re-marshalled and unmarshalled into

// the typed struct. Tolerant: nil on absence, wrong shape, or corrupt data —
// a malformed cognition must never block the deliverable write.

func extractStateCognition(v any) *DeliverableCognition {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var c DeliverableCognition

	if err := json.Unmarshal(b, &c); err != nil {
		return nil
	}
	if len(c.Decisions) == 0 && len(c.
		Rejected) ==
		0 && len(c.Assumptions) == 0 && len(c.OpenQuestions) == 0 {
		return nil
	}
	return &c
} // marshalNonReservedStateKeys serializes every deliverable state key except

// the reserved ones ("summary", "cognition") into the envelope's

// StructuredJSON field. "" when nothing remains.
func marshalNonReservedStateKeys(stateDeliv map[string]any) string {
	rest :=
		make(map[string]any, len(stateDeliv))
	for k, v := range stateDeliv {
		if k == deliverableReservedKeySummary || k == deliverableReservedKeyCognition {
			continue
		}
		if strings.HasPrefix(k, deliverableAckKeyPrefix) {
			continue
		}
		rest[k] = v
	}
	if len(rest) == 0 {
		return ""
	}
	b, err := json.
		Marshal(rest)
	if err !=
		nil {
		return ""
	}
	return string(b)
} // payloadStateKeys returns the non-reserved deliverable state keys (sorted

// for deterministic output): the reserved summary/cognition keys and the

// intra-team ack/ keys are excluded — everything else is a payload topic.

func payloadStateKeys(stateDeliv map[string]any) []string {
	keys := make([]string, 0, len(stateDeliv))
	for k := range stateDeliv {
		if k == deliverableReservedKeySummary ||
			k == deliverableReservedKeyCognition {
			continue
		}
		if strings.HasPrefix(k, deliverableAckKeyPrefix) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
} // aggregateTopicSummaries builds a readable summary from topic payloads when

// the producer did not write a top-level "summary" key (the long-article

// paradigm puts the body under contract topics). Only map-shaped topics

// contribute (title/summary/note); flat scalar keys keep the legacy

// structured-JSON fallback so no key context is lost. Returns "" when no

// topic contributes human-readable text.

func aggregateTopicSummaries(stateDeliv map[string]any) string {
	keys := payloadStateKeys(stateDeliv)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		v, ok := stateDeliv[k].(map[string]any)
		if !ok {
			continue

		}
		title, _ := v["title"].(string)
		summ,
			_ := v["summary"].(string)
		if strings.
			TrimSpace(summ) == "" {
			summ, _ = v["note"].(string)
		}
		title, summ = strings.TrimSpace(title), strings.TrimSpace(summ)
		switch {
		case title != "" && summ != "":
			lines =
				append(
					lines, "《"+title+"》"+summ)
		case
			title != "":
			lines = append(lines, "《"+
				title+
				"》")
		case summ != "":
			lines = append(lines,
				summ,
			)
		}
	}
	return TruncateRunes(
		strings.Join(lines, "\n"), MaxSummaryLen*4)
} // jsonValueRuneLen sizes a non-string payload by its indented JSON
// serialization — the same rendering extractArtifactContent/renderPayloadContent

// inline or serve, so the inline-vs-pointer threshold
// (InlineUpstreamPayloadMaxChars) measures exactly what gets injected.

func jsonValueRuneLen(v any) int {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return 0
	}
	return utf8.RuneCountInString(string(b))
} // deliverableAckKeyPrefix mirrors deliverabletools.AckKeyPrefix. biz cannot

// import internal/tools (dependency direction), so the prefix is duplicated

// and pinned by TestMarshalNonReservedStateKeys_ExcludesAckKeys.

const deliverableAckKeyPrefix = "ack/" // requiredTopicsMissingFromState is the completion-time advisory check for
// the member-level deliverable contract (MDC): topics declared required in
// the team's deliverable_contract but absent from the final deliverable map

// (covers the "producer never called set_deliverable" bypass). Advisory only

// — the caller logs a Warn and never blocks the run.

func requiredTopicsMissingFromState(t Team, stateDeliv map[string]any) []string {
	if strings.TrimSpace(t.DefinitionJSON) ==
		"" {
		return nil
	}
	var probe struct {
		DeliverableContract *MemberDeliverableContract `json:"deliverable_contract"`
	}
	if err := json.Unmarshal([]byte(t.DefinitionJSON), &probe); err != nil {
		return nil
	}
	return probe.DeliverableContract.
		RequiredTopicsMissing(stateDeliv)
} // subtractUpstreamSeed returns the entries of stateDeliv that did NOT come

// from the seed. A key counts as seeded only when its current value
// deep-equals the seed value: a member overwrite of a seeded topic changes
// the value and therefore counts as the team's OWN output. nil/empty seed

// (or empty state) returns the state unchanged.

func subtractUpstreamSeed(stateDeliv,
	seed map[string]any) map[string]any {
	if len(seed) == 0 || len(stateDeliv) == 0 {
		return stateDeliv
	}
	own := make(map[string]any,
		len(stateDeliv))
	for k, v := range stateDeliv {
		if sv, ok := seed[k]; ok && reflect.DeepEqual(sv, v) {
			continue
		}
		own[k] =
			v
	}
	return own
} // contractSubmissionLines renders the F5 per-contract submission instruction
// for the delivery-protocol suffix, one entry per contract:
//
//	契约: xlsx_install_result (data/json) — 安装结果

//	  提交方式：set_deliverable(topic="xlsx_install_result", data={...})

//
// The explicit topic removes the member's freedom to invent topic names —

// the 12:33 root cause where the spirit guessed pdf_install_result while the

// member wrote xlsx_install_result. Returns "" when no parseable contract.
func contractSubmissionLines(deliverablesJSON string) string {
	contracts, err :=
		ParseDeliverableContracts(deliverablesJSON)
	if err != nil || len(contracts) == 0 {
		return ""
	}
	lines := make([]string, 0, len(contracts)*2)
	for _, c := range contracts {
		name := strings.
			TrimSpace(c.Name)
		if name == "" {
			continue
		}
		line := fmt.Sprintf("契约: %s (%s/%s)",
			name,
			c.Type, c.Format,
		)
		if c.Description !=
			"" {
			line += " — " +

				c.Description
		}
		lines = append(lines,
			line)
		lines = append(lines, fmt.
			Sprintf("  提交方式：set_deliverable(topic=%q, data={...})",

				name))
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// contractDeclarationLines renders the P1 contract declaration for the
// injection prefix, one line per declared contract, e.g.
// "契约: research_report (document/markdown) — 调研结论报告\n".
// Returns "" when the upstream team has no parseable contract, so the
// prefix keeps its legacy shape (name + summary only).

func contractDeclarationLines(deliverablesJSON string) string {
	contracts,
		err := ParseDeliverableContracts(deliverablesJSON)
	if err != nil || len(contracts) ==
		0 {
		return ""
	}
	lines := make([]string, 0, len(contracts))
	for _, c := range contracts {
		line := fmt.Sprintf("契约: %s (%s/%s)", c.Name,
			c.Type, c.Format)
		if c.Description != "" {
			line += " — " + c.Description
		}
		lines =
			append(lines, line)
	}
	return strings.Join(lines, "\n") + "\n"
} // cognitionItemMaxRunes bounds each rendered cognition item so a verbose

// upstream record cannot blow up the injection prefix.
const cognitionItemMaxRunes = 200 // renderCognitionLines renders the C1 cognition block of an upstream

// deliverable ref for the injection prefix, one line per aspect:

//

//	[上游决策] 选择 A 方案（理由: …，置信度 0.8）；否决 B 方案（原因: …）
//	[上游假设] 数据源 Q3 已封板
//	[上游遗留问题] 样本偏差未校正
//
// Each item is truncated to cognitionItemMaxRunes. "" when cognition is nil
// or renders to nothing — legacy envelopes keep the prefix unchanged.
func renderCognitionLines(
	c *DeliverableCognition) string {
	if c == nil {
		return ""
	}
	var lines []string
	var decisions []string
	for _, d := range c.Decisions {
		item := fmt.Sprintf("选择 %s（理由: %s", d.Choice, d.Rationale)
		if d.Confidence > 0 {
			item += fmt.Sprintf("，置信度 %g", d.Confidence)
		}
		item += "）"
		decisions = append(decisions, TruncateRunes(item, cognitionItemMaxRunes))
	}
	for _, r := range c.Rejected {
		decisions = append(decisions, TruncateRunes(fmt.Sprintf("否决 %s（原因: %s）", r.Option, r.Reason), cognitionItemMaxRunes))
	}
	if len(decisions) > 0 {
		lines = append(lines, "[上游决策] "+strings.Join(decisions,
			"；"))
	}
	if len(c.Assumptions) >
		0 {
		items := make([]string, 0, len(c.
			Assumptions))
		for _, a := range c.Assumptions {
			items = append(items, TruncateRunes(a,
				cognitionItemMaxRunes))
		}
		lines = append(lines, "[上游假设] "+strings.Join(items,
			"；"))
	}
	if len(c.OpenQuestions) >
		0 {
		items := make([]string, 0, len(c.OpenQuestions))
		for _, q := range c.OpenQuestions {
			items = append(items,
				TruncateRunes(q, cognitionItemMaxRunes))
		}
		lines = append(lines,
			"[上游遗留问题] "+
				strings.Join(items, "；"))
	}
	return strings.Join(lines, "\n")
} // schemaViolationPrefix is the message prefix produced by

// shared.ValidateDocumentAgainstSchema on a document/schema mismatch; the

// mismatch record keeps only the violation detail.

const schemaViolationPrefix = "config does not match schema: "

func deliverableDocMimeExt(format string) (mime, ext string) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "html":
		return "text/html",
			".html"
	case "text",
		"txt", "plain":
		return "text/plain", ".txt"
	default:
		return "text/markdown",

			".md"
	}
}

// 的产物指针（2026-09-02 P1）。artifact_id 为空（未桥接 / workspace_rel 纯
// 指针）时 ok=false —— 无产物记录可定位，不进入 deliverables 通知。
func deliverableArtifactRefOf(art DeliverableArtifact) (DeliverableArtifactRef, bool) {
	id := strings.TrimSpace(art.ArtifactID)
	if id == "" {
		return DeliverableArtifactRef{}, false
	}
	name := strings.TrimSpace(art.Title)
	if name == "" {
		name = strings.TrimSpace(art.Key)
	}
	return DeliverableArtifactRef{
		ArtifactID: id, Name: name, Format: strings.TrimSpace(art.Format), SizeChars: art.SizeChars, MimeType: strings.TrimSpace(art.MimeType)}, true
}
