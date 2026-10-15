package biz

import (
	"aranea-agents/pkg/loggateway"

	"context"
	"encoding/json"
	"fmt"

	"strings"
	"unicode/utf8"
)

// the P2 DeliverableRef envelope needs both (SizeChars / TeamSessionID).
type teamFullOutput struct {
	Content   string // full, untruncated deliverable content
	SessionID string // team main session the content was read from
} // stateDeliverableProbe is the minimal subset of the team DefinitionJSON the
// graph-state bridge needs. biz cannot import internal/team (dependency
// direction), so the relevant fields are probed directly.
type stateDeliverableProbe struct {
	EnableStateDeliverable bool   `json:"enable_state_deliverable"`
	IntentAnchorAgentID    string `json:"intent_anchor_agent_id"`
	Members                []struct {
		AgentID string `json:"agent_id"`
	} `json:"members"`
} // anchorAgentID mirrors the runner's anchor resolution

func (p stateDeliverableProbe) anchorAgentID() string {
	first := ""
	for _, m := range p.Members {
		if id := strings.TrimSpace(m.AgentID); id != "" {
			first = id
			break
		}
	}
	if want := strings.
		TrimSpace(p.IntentAnchorAgentID); want !=
		"" {
		for _, m := range p.Members {
			if strings.
				TrimSpace(m.AgentID) == want {
				return want
			}
		}
	}
	return first
}

func (d *SpiritDelivery) bridgeDeliverableDocs(ctx context.Context, t Team, stateDeliv map[string]any) {
	if d.artifactSaver ==
		nil ||
		strings.TrimSpace(t.
			SpiritSessionID,
		) == "" {
		return
	}
	for _, k := range payloadStateKeys(stateDeliv) {
		v, ok := stateDeliv[k].(map[string]any)
		if !ok {
			continue
		}
		content, _ := v["content"].(string)
		if strings.TrimSpace(content) == "" {
			continue
		}
		if id, _ := v["artifact_id"].(string); strings.TrimSpace(id) != "" {
			continue
		}
		title, _ := v["title"].(string)
		if title = strings.TrimSpace(title); title == "" {
			title = k
		}
		format,

			_ := v["format"].(string)
		mime, ext := deliverableDocMimeExt(format)
		saved, err := d.artifactSaver.Save(ctx, t.SpiritSessionID, title+ext, mime, []byte(content))
		if err != nil {
			d.lg.Warn("交付物文档桥接 artifact 失败（信封仍按原样落库）",

				loggateway.StepID("spirit.write_deliverables.artifact_bridge"), loggateway.Str("team_id", t.ID), loggateway.Str("topic", k),
				loggateway.
					Err(err))
			continue
		}
		v["artifact_id"] = saved.ID
		v["artifact_rel"] = saved.StorageURI
		d.lg.Info("交付物文档已桥接为会话产物", loggateway.
			StepID("spirit.write_deliverables.artifact_bridge"), loggateway.
			Str("team_id", t.ID), loggateway.Str("topic",
			k), loggateway.Str("artifact_id", saved.ID), loggateway.Str("storage_uri", saved.StorageURI))
	}
}

func buildDeliverableArtifacts(stateDeliv map[string]any, deliverablesJSON string) []DeliverableArtifact {
	keys := payloadStateKeys(stateDeliv)
	if len(keys) == 0 {
		return nil
	}
	contracts,
		_ := ParseDeliverableContracts(deliverablesJSON)
	byName := make(map[string]DeliverableContract,

		len(contracts))
	for _, c := range contracts {
		byName[strings.
			TrimSpace(c.Name)] = c
	}
	out := make([]DeliverableArtifact,
		0, len(keys))
	for _, k := range keys {
		art :=
			DeliverableArtifact{Key: k}
		if c, ok := byName[k]; ok {
			art.Type = strings.
				TrimSpace(c.Type)
			art.Format = strings.TrimSpace(c.Format)
		}
		switch v := stateDeliv[k].(type) {
		case map[string]any:
			fillArtifactFromPayload(&art, v)
		case string:
			art.SizeChars = utf8.RuneCountInString(v)
		default:
			art.SizeChars = jsonValueRuneLen(v)
		}
		out = append(out, art)
	}
	return out
}
func fillArtifactFromPayload(art *DeliverableArtifact, v map[string]any) {
	if art ==
		nil || v == nil {
		return
	}
	if title, ok := v["title"].(string); ok {
		art.Title = strings.
			TrimSpace(title)
	}
	if art.Format == "" {
		if f,
			ok := v["format"].(string); ok {
			art.Format = strings.TrimSpace(f)
		}
	}
	if mime, ok := v["mime_type"].(string); ok {
		art.MimeType = strings.
			TrimSpace(mime)
	}
	if sha, ok := v["sha256"].(string); ok {
		art.SHA256 = strings.TrimSpace(sha)
	}
	if id, ok := v["artifact_id"].(string); ok {
		art.ArtifactID = strings.TrimSpace(id)
	}
	if su, ok := v["artifact_rel"].(string); ok {
		art.StorageURI = strings.TrimSpace(su)
	}
	if rel, ok := v["rel_path"].(string); ok {
		art.RelPath = sanitizeInboxRelPath(rel)
	} else if rel, ok := v["path"].(string); ok {
		art.RelPath = sanitizeInboxRelPath(
			rel,
		)
	}
	switch n := v["size_bytes"].(type) {
	case float64:
		art.SizeBytes = int64(
			n)
	case int:
		art.SizeBytes = int64(n)
	case int64:
		art.
			SizeBytes = n
	}
	if content,
		ok := v["content"].(string); ok && strings.TrimSpace(content) != "" {
		art.SizeChars = utf8.RuneCountInString(content)
	} else if art.SizeChars == 0 {
		art.SizeChars = jsonValueRuneLen(v)
	}
	bridged := strings.TrimSpace(art.StorageURI) != ""
	hasContent := false
	if content,

		ok := v["content"].(string); ok {
		hasContent = strings.TrimSpace(content) != ""
	}
	switch {
	case bridged && hasContent:
	case strings.TrimSpace(art.ArtifactID) != "":
		art.Kind = DeliverableArtifactKindArtifact
	case strings.
		TrimSpace(art.RelPath) != "":
		art.Kind = DeliverableArtifactKindWorkspaceRel
	}
}

// Bulk pointers and oversize text omit the full content (M78 ORGFAST-16).
func marshalEnvelopeStructuredJSON(stateDeliv map[string]any, artifacts []DeliverableArtifact) string {
	byKey := make(map[string]DeliverableArtifact,
		len(artifacts))
	for _, a := range artifacts {
		byKey[a.Key] = a
	}
	rest := make(map[string]any,
		len(stateDeliv))
	for k, v := range stateDeliv {
		if k == deliverableReservedKeySummary ||
			k == deliverableReservedKeyCognition {
			continue
		}
		if strings.HasPrefix(k, deliverableAckKeyPrefix) {
			continue
		}
		art := byKey[k]
		if art.IsBulkPointer() {
			rest[k] = bulkPointerStub(v, art)
			continue
		}
		if art.SizeChars > MaxEnvelopeStructuredPayloadChars {
			rest[k] =

				oversizedTextStub(v, art)
			continue
		}
		rest[k] = v
	}
	if len(rest) == 0 {
		return ""
	}
	b, err := json.Marshal(rest)
	if err != nil {
		return ""
	}
	return string(b)
}
func bulkPointerStub(v any, art DeliverableArtifact) map[string]any {
	stub := map[string]any{"content_omitted": true, "kind": art.ResolvedKind()}
	if art.Title != "" {
		stub["title"] = art.Title
	}
	if art.Format !=
		"" {
		stub["format"] = art.Format
	}
	if art.RelPath != "" {
		stub["rel_path"] = art.
			RelPath
	}
	if art.ArtifactID != "" {
		stub["artifact_id"] = art.ArtifactID
	}
	if art.SizeBytes > 0 {
		stub["size_bytes"] = art.SizeBytes
	}
	if m, ok := v.(map[string]any); ok {
		if t, ok := m["title"].(string); ok && stub["title"] == nil {
			stub["title"] = t
		}
	}
	return stub
}
func oversizedTextStub(v any, art DeliverableArtifact) map[string]any {
	stub := map[string]any{"content_omitted": true, "size_chars": art.SizeChars}
	if m, ok := v.(map[string]any); ok {
		if t,
			ok := m["title"].(string); ok {
			stub["title"] = t
		}
		if f, ok :=
			m["format"].(string); ok {
			stub["format"] = f
		}
	}
	if art.Title != "" {
		stub["title"] = art.Title
	}
	if art.Format != "" {
		stub["format"] = art.Format
	}
	return stub
}

// line, e.g. article《云计算十年》（markdown，8234字）.
// 已桥接 artifact 的文档追加用户可定位路径（publicBase 未配置时省略）。
func (d *SpiritDelivery) artifactDigestLine(art DeliverableArtifact) string {
	label := art.Key
	if art.Title != "" {
		label += "《" + art.Title + "》"
	}
	format := art.Format
	if format == "" {
		format = "text"
	}
	line := fmt.
		Sprintf("%s（%s，%d字）",

			label, format, art.SizeChars)
	if p :=
		d.artifactPublicPath(art); p !=
		"" {
		line += "，路径：" +
			p
	}
	return line
}

func (d *SpiritDelivery) artifactPublicPath(art DeliverableArtifact) string {
	base := strings.TrimSpace(d.artifactPublicBase)
	rel := strings.TrimSpace(art.StorageURI)
	if base ==
		"" || rel == "" {
		return ""
	}
	return base + `\` + strings.ReplaceAll(rel,
		"/", `\`)
}

// renderArtifactSections renders the upstream envelope's payload artifacts:
// small payloads (≤ InlineUpstreamPayloadMaxChars) inline in full so the
// downstream team consumes them with zero tool calls; larger payloads render
// as a pointer plus a keyed read_upstream_deliverable instruction. Inline
// rendering is bounded by InlineUpstreamArtifactMaxCount and
// InlineUpstreamPayloadTotalMaxChars — once either budget is spent, remaining
// payloads degrade to pointers so a multi-topic deliverable cannot bloat the
// downstream first-turn input. Empty payloads (SizeChars == 0) render as an
// honest note WITHOUT a retrieval instruction: a keyed read of them can only
// return "内容为空". When the artifact key hits the downstream team's
// InputContract, the retrieval instruction escalates from an optional hint to
// a mandatory pre-task step — fetching the full text no longer depends on LLM
// discretion.
func renderArtifactSections(ref DeliverableRef, downstreamTeam Team) string {
	if len(ref.Artifacts) == 0 {
		return ""
	}
	inputNames := make(map[string]bool)
	if contracts, err := ParseDeliverableContracts(downstreamTeam.InputContract); err == nil {
		for _, c := range contracts {
			if name := strings.TrimSpace(c.Name); name != "" {
				inputNames[name] = true
			}
		}
	}
	// structured parses lazily on the first inline decision.
	var structured map[string]any
	structuredParsed := false
	parseStructured := func() map[string]any {
		if !structuredParsed {
			structuredParsed = true
			if ref.StructuredJSON != "" {
				_ = json.Unmarshal([]byte(ref.StructuredJSON), &structured)
			}
		}
		return structured
	}

	var sb strings.Builder
	inlinedCount, inlinedChars := 0, 0
	for _, art := range ref.Artifacts {
		label := art.Key
		if art.Title != "" {
			label += "《" + art.Title + "》"
		}
		format := art.Format
		if format == "" {
			format = "text"
		}
		if art.IsBulkPointer() {
			dest := art.RelPath
			if dest == "" {
				dest = art.Key
			}
			sizeHint := ""
			if art.SizeBytes > 0 {
				sizeHint = fmt.Sprintf("，%d 字节", art.SizeBytes)
			} else if art.SizeChars > 0 {
				sizeHint = fmt.Sprintf("，%d 字", art.SizeChars)
			}
			sb.WriteString(fmt.Sprintf("\n- 附件 %s（%s%s）已物化到 inbox/%s/%s，请用文件工具按需读取，禁止把文件正文贴进回复。",
				label, format, sizeHint, ref.TeamID, dest))
			continue
		}
		if art.SizeChars == 0 {

			sb.WriteString(fmt.Sprintf("\n- 交付物 %s（%s）：载荷内容为空（上游写入但无正文）。", label, format))
			continue
		}
		withinInlineBudget := inlinedCount < InlineUpstreamArtifactMaxCount &&
			inlinedChars+art.SizeChars <= InlineUpstreamPayloadTotalMaxChars
		if art.SizeChars <= InlineUpstreamPayloadMaxChars && withinInlineBudget {
			content := extractArtifactContent(parseStructured(), art.Key)
			if content == "" {

				sb.WriteString(fmt.Sprintf("\n- 交付物 %s（%s，共 %d 字）。需要全文时调用 read_upstream_deliverable(team_id=\"%s\", key=\"%s\") 获取。",
					label, format, art.SizeChars, ref.TeamID, art.Key))
				continue
			}
			inlinedCount++
			inlinedChars += art.SizeChars
			sb.WriteString(fmt.Sprintf("\n### 交付物 %s（%s，全文 %d 字）\n%s", label, format, art.SizeChars, content))
			continue
		}

		call := fmt.Sprintf("read_upstream_deliverable(team_id=\"%s\", key=\"%s\")", ref.TeamID, art.Key)
		if art.SizeChars > DefaultUpstreamDeliverableMaxChars {
			budget := art.SizeChars
			if budget > MaxUpstreamDeliverableChars {
				budget = MaxUpstreamDeliverableChars
			}
			call = fmt.Sprintf("read_upstream_deliverable(team_id=\"%s\", key=\"%s\", max_chars=%d)", ref.TeamID, art.Key, budget)
		}
		sb.WriteString(fmt.Sprintf("\n- 交付物 %s（%s，共 %d 字，全文未内联）", label, format, art.SizeChars))
		if inputNames[art.Key] {
			sb.WriteString(fmt.Sprintf("\n  你的输入契约包含 %s：开始任务前必须先调用 %s 获取全文，再基于全文执行。",
				art.Key, call))
		} else {
			sb.WriteString(fmt.Sprintf("\n  需要全文时调用 %s 获取。", call))
		}
	}
	return sb.String()
}

// extractArtifactContent pulls one payload's body out of the envelope's
// StructuredJSON: document-paradigm maps contribute their "content" field,
// plain strings pass through, other shapes serialize as indented JSON.
func extractArtifactContent(structured map[string]any, key string) string {
	if structured == nil {
		return ""
	}
	v, ok := structured[key]
	if !ok {
		return ""
	}
	switch payload := v.(type) {
	case map[string]any:
		if content, ok := payload["content"].(string); ok {
			return strings.TrimSpace(content)
		}
		b, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return ""
		}
		return string(b)
	case string:
		return strings.TrimSpace(payload)
	default:
		b, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return ""
		}
		return string(b)
	}
}
