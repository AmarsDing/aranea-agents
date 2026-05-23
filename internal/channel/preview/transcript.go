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

	segmentTextID       = "__text__"
	segmentReasoningID  = "__reasoning__"
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
func (t *Transcript) Apply(env event.Envelope) {
	switch env.Type {
	case event.EnvelopeTypeTextDelta:
		if env.Content == nil {
			return
		}
		if s := env.Content.Reasoning; strings.TrimSpace(s) != "" {
			t.appendText(segmentReasoningID, s, env.Content.IsPartial)
		}
		if env.Content.Text != "" {
			t.appendText(segmentTextID, env.Content.Text, env.Content.IsPartial)
		}
	case event.EnvelopeTypeTextDone:
		if env.Content == nil {
			return
		}
		if s := strings.TrimSpace(env.Content.Reasoning); s != "" {
			t.setText(segmentReasoningID, s)
		}
		if s := strings.TrimSpace(env.Content.Text); s != "" {
			t.setText(segmentTextID, s)
		}
	case event.EnvelopeTypeToolCall:
		if env.ToolCall == nil {
			return
		}
		tc := env.ToolCall
		status := NormalizeToolStatus(tc.Status)
		if status == "" {
			status = ToolStatusCalling
		}
		seg := Segment{
			Kind:   SegmentTool,
			ID:     strings.TrimSpace(tc.ID),
			Status: status,
			Meta:   toolMetaFromEnvelope(tc),
		}
		if seg.ID == "" {
			seg.ID = seg.Meta.Name
		}
		if idx, ok := t.index[seg.ID]; ok {
			t.segments[idx].Status = status
			t.segments[idx].Meta = mergeToolMeta(t.segments[idx].Meta, tc)
			return
		}
		t.breakTextSegment()
		t.breakReasoningSegment()
		t.append(seg)
	case event.EnvelopeTypeToolResult:
		if env.ToolCall == nil {
			return
		}
		id := strings.TrimSpace(env.ToolCall.ID)
		if id == "" {
			id = strings.TrimSpace(env.ToolCall.Name)
		}
		status := NormalizeToolStatus(env.ToolCall.Status)
		if status == "" {
			status = ToolStatusOK
		}
		if idx, ok := t.index[id]; ok {
			t.segments[idx].Status = status
			t.segments[idx].Meta = mergeToolMeta(t.segments[idx].Meta, env.ToolCall)
			return
		}
		t.append(Segment{
			Kind:   SegmentTool,
			ID:     id,
			Status: status,
			Meta:   toolMetaFromEnvelope(env.ToolCall),
		})
	case event.EnvelopeTypeMemberMessageStart, event.EnvelopeTypeMemberDelta:
		author := strings.TrimSpace(env.Author)
		if author == "" {
			return
		}
		text := ""
		if env.Content != nil {
			text = strings.TrimSpace(env.Content.Text)
		}
		if text == "" && env.Type == event.EnvelopeTypeMemberMessageStart {
			return
		}
		t.appendMember(author, text, env.Type == event.EnvelopeTypeMemberDelta)
	case event.EnvelopeTypeMemberMessageDone:
		author := strings.TrimSpace(env.Author)
		if author == "" || env.Content == nil {
			return
		}
		t.setMember(author, strings.TrimSpace(env.Content.Text))
	}
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
