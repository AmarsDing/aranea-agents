package preview

import (
	"strings"
)

type SegmentKind string

const (
	SegmentReasoning SegmentKind = "reasoning"
	SegmentText      SegmentKind = "text"
	SegmentTool      SegmentKind = "tool"
	SegmentMember    SegmentKind = "member"
	SegmentSystem    SegmentKind = "system"
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

// Transcript accumulates segments for IM rendering.
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

func (t *Transcript) append(seg Segment) {
	t.index[seg.ID] = len(t.segments)
	t.segments = append(t.segments, seg)
}

func (t *Transcript) Segments() []Segment {
	out := make([]Segment, len(t.segments))
	copy(out, t.segments)
	return out
}
