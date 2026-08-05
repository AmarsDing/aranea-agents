package team

import (
	"sync"

	"aranea-agents/pkg/loggateway"
)

// GraphRunStepContext is the public DTO for graph run step persistence (ARCH-01).
// Keeps the finisher decoupled from coordinator session internals.
type GraphRunStepContext struct {
	TeamRunID       string
	TeamID          string
	SessionID       string
	SpiritSessionID string
	// RootTaskID is the run-dimension captured at registration (S-3); the
	// finisher must derive the team_stage Activity ID from it rather than
	// the triggering ctx, which never carries RootTaskActivityID on the
	// resume/finalize path.
	RootTaskID    string
	InputPreview  string
	memberByNode  map[string]MemberDef
	stepSortIndex map[string]int
	dedup         *graphStepDedup
}

type graphStepDedup struct {
	mu    sync.Mutex
	nodes map[string]struct{}
}

func newGraphStepDedup() *graphStepDedup {
	return &graphStepDedup{nodes: make(map[string]struct{})}
}

func (d *graphStepDedup) mark(nodeID string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.nodes == nil {
		d.nodes = make(map[string]struct{})
	}
	d.nodes[nodeID] = struct{}{}
}

func (d *graphStepDedup) has(nodeID string) bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.nodes[nodeID]
	return ok
}

func (s *teamGraphRunSession) stepContext() *GraphRunStepContext {
	if s == nil {
		return nil
	}
	if s.stepDedup == nil {
		s.stepDedup = newGraphStepDedup()
	}
	return &GraphRunStepContext{
		TeamRunID:       s.teamRunID,
		TeamID:          s.teamID,
		SessionID:       s.sessionID,
		SpiritSessionID: s.spiritSessionID,
		RootTaskID:      s.rootTaskID,
		InputPreview:    s.inputPreview,
		memberByNode:    s.memberByNode,
		stepSortIndex:   s.stepSortIndex,
		dedup:           s.stepDedup,
	}
}

func buildGraphRunStepContext(defJSON, inputPreview, teamRunID, teamID, sessionID, spiritSessionID string, lg loggateway.Logger) *GraphRunStepContext {
	_, memberByNode, stepSortIndex := buildResumeSessionContext(defJSON, inputPreview, nil, lg)
	return &GraphRunStepContext{
		TeamRunID:       teamRunID,
		TeamID:          teamID,
		SessionID:       sessionID,
		SpiritSessionID: spiritSessionID,
		InputPreview:    inputPreview,
		memberByNode:    memberByNode,
		stepSortIndex:   stepSortIndex,
		dedup:           newGraphStepDedup(),
	}
}

// MemberDefForNode returns the member definition for a compiled graph node id.
func (c *GraphRunStepContext) MemberDefForNode(nodeID string) (MemberDef, bool) {
	if c == nil {
		return MemberDef{}, false
	}
	m, ok := c.memberByNode[nodeID]
	return m, ok
}

func (c *GraphRunStepContext) SortIndex(nodeID string) int {
	if c == nil {
		return 0
	}
	return c.stepSortIndex[nodeID]
}

func (c *GraphRunStepContext) MarkPersisted(nodeID string) {
	if c == nil || c.dedup == nil {
		return
	}
	c.dedup.mark(nodeID)
}

func (c *GraphRunStepContext) AlreadyPersisted(nodeID string) bool {
	if c == nil {
		return false
	}
	return c.dedup.has(nodeID)
}
