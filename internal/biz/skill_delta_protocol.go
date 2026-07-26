package biz

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"aranea-agents/pkg/apierror"
)

// ── Delta 更新协议（P1）───────────────────────────────────────────────────────
//
// SKILL.md 正文中的可操作规则用 HTML 注释标记包裹为「规则块」：
//
//	<!-- aranea:rule id="timeout-retry" helpful=3 harmful=1 -->
//	当工具调用超时时，先指数退避重试一次，再降级到备选方案。
//	<!-- /aranea:rule -->
//
// Curator 在 delta 模式下只输出操作序列（add/modify/merge/remove），由程序
// 执行局部更新，避免整体替换导致的 Context Collapse（丢失触发条件、失败
// 模式等局部信息）。规则块的 id 跨版本保持稳定，是计数归因的锚点；
// helpful/harmful 计数随正文在版本间传递（只在新版本注册时更新，不就地
// 改写旧版本，保持版本不可变）。

var (
	// ruleStartRe matches the rule block opening marker, capturing id/helpful/harmful.
	ruleStartRe = regexp.MustCompile(`^<!--\s*aranea:rule\s+id="([^"]+)"((?:\s+(?:helpful|harmful)=\d+)*)\s*-->\s*$`)
	// ruleEndRe matches the rule block closing marker.
	ruleEndRe = regexp.MustCompile(`^<!--\s*/aranea:rule\s*-->\s*$`)
	// ruleAttrRe parses a single helpful=N / harmful=N attribute pair.
	ruleAttrRe = regexp.MustCompile(`(helpful|harmful)=(\d+)`)
	// ruleBlockPresenceRe is a cheap presence check (HasRuleBlocks).
	ruleBlockPresenceRe = regexp.MustCompile(`<!--\s*aranea:rule\s+id="[^"]+"`)
)

// Delta op names.
const (
	DeltaOpAdd    = "add"
	DeltaOpModify = "modify"
	DeltaOpMerge  = "merge"
	DeltaOpRemove = "remove"
)

// RuleBlock is one actionable rule with a stable ID and attribution counters.
type RuleBlock struct {
	ID      string
	Helpful int
	Harmful int
	Content string
}

// docElem is one ordered element of a parsed document: either a non-rule
// segment (rule == nil) or a rule block.
type docElem struct {
	segment string
	rule    *RuleBlock
}

// RuleDocument is a parsed SKILL.md body: non-rule segments and rule blocks
// in original order. Render() round-trips back to markdown.
type RuleDocument struct {
	elems []docElem
}

// HasRuleBlocks reports whether the body contains at least one rule block
// opening marker. Cheap regex check; does not validate block structure.
func HasRuleBlocks(body string) bool {
	return ruleBlockPresenceRe.MatchString(body)
}

// ParseRuleBlocks parses a SKILL.md body into a RuleDocument. Lenient by
// design: an unterminated opening marker is treated as ordinary text (it is
// just an HTML comment to renderers), never an error.
func ParseRuleBlocks(body string) *RuleDocument {
	doc := &RuleDocument{}
	var segmentLines []string
	lines := strings.Split(body, "\n")

	flushSegment := func() {
		if len(segmentLines) == 0 {
			return
		}
		doc.elems = append(doc.elems, docElem{segment: strings.Join(segmentLines, "\n")})
		segmentLines = nil
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		m := ruleStartRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			segmentLines = append(segmentLines, line)
			continue
		}
		// Collect content until the closing marker.
		var contentLines []string
		closed := false
		for j := i + 1; j < len(lines); j++ {
			if ruleEndRe.MatchString(strings.TrimSpace(lines[j])) {
				i = j
				closed = true
				break
			}
			contentLines = append(contentLines, lines[j])
		}
		if !closed {
			// Unterminated marker → treat the marker line as ordinary text.
			segmentLines = append(segmentLines, line)
			continue
		}
		flushSegment()
		block := &RuleBlock{
			ID:      m[1],
			Content: strings.TrimSpace(strings.Join(contentLines, "\n")),
		}
		for _, attr := range ruleAttrRe.FindAllStringSubmatch(m[2], -1) {
			n, _ := strconv.Atoi(attr[2])
			if attr[1] == "helpful" {
				block.Helpful = n
			} else {
				block.Harmful = n
			}
		}
		doc.elems = append(doc.elems, docElem{rule: block})
	}
	flushSegment()
	return doc
}

// Render serializes the document back to markdown. Counters are written back
// into the opening markers only when non-zero, keeping new blocks clean.
func (d *RuleDocument) Render() string {
	var b strings.Builder
	first := true
	write := func(s string) {
		if !first {
			b.WriteString("\n")
		}
		b.WriteString(s)
		first = false
	}
	for _, e := range d.elems {
		if e.rule == nil {
			write(e.segment)
			continue
		}
		var marker strings.Builder
		marker.WriteString(`<!-- aranea:rule id="`)
		marker.WriteString(e.rule.ID)
		marker.WriteString(`"`)
		if e.rule.Helpful > 0 {
			marker.WriteString(" helpful=")
			marker.WriteString(strconv.Itoa(e.rule.Helpful))
		}
		if e.rule.Harmful > 0 {
			marker.WriteString(" harmful=")
			marker.WriteString(strconv.Itoa(e.rule.Harmful))
		}
		marker.WriteString(" -->")
		write(marker.String() + "\n" + e.rule.Content + "\n<!-- /aranea:rule -->")
	}
	return b.String()
}

// Rules returns the rule blocks in document order.
func (d *RuleDocument) Rules() []*RuleBlock {
	var out []*RuleBlock
	for _, e := range d.elems {
		if e.rule != nil {
			out = append(out, e.rule)
		}
	}
	return out
}

// RuleByID returns the rule with the given ID, or nil.
func (d *RuleDocument) RuleByID(id string) *RuleBlock {
	for _, e := range d.elems {
		if e.rule != nil && e.rule.ID == id {
			return e.rule
		}
	}
	return nil
}

// DeltaOp is one local update instruction produced by the Curator.
type DeltaOp struct {
	Op      string `json:"op"`
	RuleID  string `json:"rule_id"`
	Content string `json:"content,omitempty"`
}

// ParseDeltaOpsJSON parses the Curator's delta-mode output. Tolerates a
// surrounding ```json / ``` code fence. Every op is validated; any invalid
// entry rejects the whole delta (callers fall back to full-rewrite mode).
func ParseDeltaOpsJSON(text string) ([]DeltaOp, error) {
	raw := strings.TrimSpace(text)
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if n := len(lines); n > 0 && strings.TrimSpace(lines[n-1]) == "```" {
				lines = lines[:n-1]
			}
			raw = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	if raw == "" {
		return nil, apierror.BadRequest("DELTA_PROTOCOL", "empty delta ops")
	}
	var ops []DeltaOp
	if err := json.Unmarshal([]byte(raw), &ops); err != nil {
		return nil, apierror.BadRequest("DELTA_PROTOCOL", "invalid delta ops JSON: %s", err)
	}
	if len(ops) == 0 {
		return nil, apierror.BadRequest("DELTA_PROTOCOL", "delta ops array is empty")
	}
	for i, op := range ops {
		op.RuleID = strings.TrimSpace(op.RuleID)
		ops[i] = op
		switch op.Op {
		case DeltaOpAdd, DeltaOpModify, DeltaOpMerge, DeltaOpRemove:
		default:
			return nil, apierror.BadRequest("DELTA_PROTOCOL", "op[%d]: unknown op %q", i, op.Op)
		}
		if op.RuleID == "" {
			return nil, apierror.BadRequest("DELTA_PROTOCOL", "op[%d]: rule_id is required", i)
		}
		if op.Op != DeltaOpRemove && strings.TrimSpace(op.Content) == "" {
			return nil, apierror.BadRequest("DELTA_PROTOCOL", "op[%d]: content is required for %q", i, op.Op)
		}
	}
	return ops, nil
}

// ApplyDeltaOps applies ops to the document in order. Strict semantics:
// modify/merge/remove must reference an existing rule, add must reference a
// new ID, and the document must not contain duplicate IDs — any violation
// rejects the whole delta (callers fall back to full-rewrite mode).
// Returns the IDs of rules changed by the ops.
func ApplyDeltaOps(doc *RuleDocument, ops []DeltaOp) ([]string, error) {
	var changed []string
	for i, op := range ops {
		switch op.Op {
		case DeltaOpAdd:
			if doc.RuleByID(op.RuleID) != nil {
				return nil, apierror.BadRequest("DELTA_PROTOCOL", "op[%d]: add duplicates existing rule %q", i, op.RuleID)
			}
			doc.elems = append(doc.elems, docElem{rule: &RuleBlock{ID: op.RuleID, Content: strings.TrimSpace(op.Content)}})
			changed = append(changed, op.RuleID)
		case DeltaOpModify:
			r := doc.RuleByID(op.RuleID)
			if r == nil {
				return nil, apierror.BadRequest("DELTA_PROTOCOL", "op[%d]: modify references unknown rule %q", i, op.RuleID)
			}
			r.Content = strings.TrimSpace(op.Content)
			changed = append(changed, op.RuleID)
		case DeltaOpMerge:
			r := doc.RuleByID(op.RuleID)
			if r == nil {
				return nil, apierror.BadRequest("DELTA_PROTOCOL", "op[%d]: merge references unknown rule %q", i, op.RuleID)
			}
			r.Content = strings.TrimSpace(r.Content + "\n" + strings.TrimSpace(op.Content))
			changed = append(changed, op.RuleID)
		case DeltaOpRemove:
			idx := -1
			for j, e := range doc.elems {
				if e.rule != nil && e.rule.ID == op.RuleID {
					idx = j
					break
				}
			}
			if idx < 0 {
				return nil, apierror.BadRequest("DELTA_PROTOCOL", "op[%d]: remove references unknown rule %q", i, op.RuleID)
			}
			doc.elems = append(doc.elems[:idx], doc.elems[idx+1:]...)
			changed = append(changed, op.RuleID)
		}
	}
	return changed, nil
}

// BumpRuleCounters increments helpful or harmful counters on the given rule
// IDs (rules that no longer exist are skipped). Called before delta
// application to settle the previous cycle's attribution verdict onto the
// rules it touched; the updated counters persist via the new version's body.
func BumpRuleCounters(doc *RuleDocument, ruleIDs []string, verdict string) {
	for _, id := range ruleIDs {
		r := doc.RuleByID(id)
		if r == nil {
			continue
		}
		switch verdict {
		case EvoEffectivenessHelpful:
			r.Helpful++
		case EvoEffectivenessHarmful:
			r.Harmful++
		}
	}
}

// Attribution verdicts for EvoMetaEffectiveness.
const (
	EvoEffectivenessHelpful          = "helpful"
	EvoEffectivenessHarmful          = "harmful"
	EvoEffectivenessNeutral          = "neutral"
	EvoEffectivenessInsufficientData = "insufficient_data"
)
