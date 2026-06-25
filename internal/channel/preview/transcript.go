package preview

import (
	"fmt"
	"strings"

	"aranea-agents/internal/event"
)

type SegmentKind string

const (
	SegmentReasoning SegmentKind = "reasoning"
	SegmentText      SegmentKind = "text"
	SegmentTool      SegmentKind = "tool"
	SegmentMember    SegmentKind = "member"
	SegmentSystem    SegmentKind = "system"

	segmentTextID      = "__text__"
	segmentReasoningID = "__reasoning__"
)

// ToolSegmentMeta carries display metadata for tool/MCP/skill segments.
type ToolSegmentMeta struct {
	ActivityKind  string
	DisplayLabel  string
	Summary       string
	Name          string
	DurationMS    int64
	ErrorCode     string
	ResultExcerpt string
}

// Segment is one ordered unit in a turn transcript.
type Segment struct {
	Kind    SegmentKind
	ID      string
	Content string
	Status  string
	Meta    ToolSegmentMeta
	Author  string
}

// Transcript accumulates Envelope events in arrival order for IM rendering.
type Transcript struct {
	segments []Segment
	index    map[string]int
}

func NewTranscript() *Transcript {
	return &Transcript{index: make(map[string]int)}
}

// SetSystem replaces or inserts a system segment (e.g. ACK placeholder).
func (t *Transcript) SetSystem(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if idx, ok := t.index["__system__"]; ok {
		t.segments[idx].Content = text
		return
	}
	t.append(Segment{Kind: SegmentSystem, ID: "__system__", Content: text})
}

// Apply updates transcript state from a chat Envelope.
//
// Phase 1c-5: All chat content envelope types (TextDelta/TextDone/ToolCall/
// ToolResult/MemberMessage*) were deleted — replaced by the Activity-First
// pipeline. This method is now a no-op; the Transcript struct is retained
// because the IM channel preview coordinator still uses it for system ACK
// placeholders. Phase 3-4 will rebuild IM rendering on ActivityEventBus.
func (t *Transcript) Apply(env event.Envelope) {
	_ = env
}

func (t *Transcript) appendText(id, piece string, partial bool) {
	if piece == "" {
		return
	}
	if idx, ok := t.index[id]; ok {
		if partial {
			t.segments[idx].Content += piece
		} else {
			t.segments[idx].Content = piece
		}
		return
	}
	t.append(Segment{Kind: segmentKindForTextID(id), ID: id, Content: piece})
}

func (t *Transcript) setText(id, text string) {
	if text == "" {
		return
	}
	if idx, ok := t.index[id]; ok {
		t.segments[idx].Content = text
		return
	}
	t.append(Segment{Kind: segmentKindForTextID(id), ID: id, Content: text})
}

func (t *Transcript) appendMember(author, piece string, appendContent bool) {
	key := "member:" + author
	if idx, ok := t.index[key]; ok {
		if appendContent && piece != "" {
			t.segments[idx].Content += piece
		} else if piece != "" {
			t.segments[idx].Content = piece
		}
		return
	}
	t.append(Segment{Kind: SegmentMember, ID: key, Author: author, Content: piece})
}

func (t *Transcript) setMember(author, text string) {
	key := "member:" + author
	if idx, ok := t.index[key]; ok {
		t.segments[idx].Content = text
		return
	}
	t.append(Segment{Kind: SegmentMember, ID: key, Author: author, Content: text})
}

func (t *Transcript) HasInFlightTool() bool {
	for _, seg := range t.segments {
		if seg.Kind != SegmentTool {
			continue
		}
		if ToolStatusInFlight(seg.Status) {
			return true
		}
	}
	return false
}

func (t *Transcript) append(seg Segment) {
	t.index[seg.ID] = len(t.segments)
	t.segments = append(t.segments, seg)
}

func (t *Transcript) Segments() []Segment {
	out := make([]Segment, len(t.segments))
	copy(out, t.segments)
	return out
}

// breakTextSegment finalizes the open text segment so later text starts a new block after tools.
func (t *Transcript) breakTextSegment() {
	t.breakSegmentID(segmentTextID)
}

func (t *Transcript) breakReasoningSegment() {
	t.breakSegmentID(segmentReasoningID)
}

func (t *Transcript) breakSegmentID(id string) {
	idx, ok := t.index[id]
	if !ok {
		return
	}
	if strings.TrimSpace(t.segments[idx].Content) == "" {
		delete(t.index, id)
		return
	}
	newID := fmt.Sprintf("%s:%d", id, len(t.segments))
	t.segments[idx].ID = newID
	t.index[newID] = idx
	delete(t.index, id)
}

func segmentKindForTextID(id string) SegmentKind {
	if id == segmentReasoningID {
		return SegmentReasoning
	}
	return SegmentText
}

func toolMetaFromEnvelope(tc *event.EnvelopeToolCall) ToolSegmentMeta {
	if tc == nil {
		return ToolSegmentMeta{}
	}
	meta := ToolSegmentMeta{
		ActivityKind: strings.TrimSpace(tc.ActivityKind),
		DisplayLabel: strings.TrimSpace(tc.DisplayLabel),
		Summary:      strings.TrimSpace(tc.Summary),
		Name:         strings.TrimSpace(tc.Name),
		DurationMS:   tc.DurationMS,
		ErrorCode:    strings.TrimSpace(tc.ErrorCode),
	}
	if tc.ResultJSON != "" {
		meta.ResultExcerpt = excerptToolResult(tc.ResultJSON, tc.Status, cardResultExcerptRunes)
	}
	return meta
}

func mergeToolMeta(existing ToolSegmentMeta, tc *event.EnvelopeToolCall) ToolSegmentMeta {
	incoming := toolMetaFromEnvelope(tc)
	if incoming.DisplayLabel != "" {
		existing.DisplayLabel = incoming.DisplayLabel
	}
	if incoming.Summary != "" {
		existing.Summary = incoming.Summary
	}
	if incoming.Name != "" {
		existing.Name = incoming.Name
	}
	if incoming.ActivityKind != "" {
		existing.ActivityKind = incoming.ActivityKind
	}
	if incoming.DurationMS > 0 {
		existing.DurationMS = incoming.DurationMS
	}
	if incoming.ErrorCode != "" {
		existing.ErrorCode = incoming.ErrorCode
	}
	if incoming.ResultExcerpt != "" {
		existing.ResultExcerpt = incoming.ResultExcerpt
	}
	return existing
}
