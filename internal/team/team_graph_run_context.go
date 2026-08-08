package team

import (
	"strings"
	"sync"
	"time"

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
	// nodeStarts tracks per-node execution start timestamps (2026-08-08
	// 问题4b): node_start watch 时记录，node_end persistStep 时取出计算真实
	// DurationMS。必须挂在共享 tracker 上——handleGraphWatchNotice 每条
	// notice 都新建 stepCtx，只有 session 级共享才能跨 notice 传递。
	nodeStarts *graphNodeStartTracker
}

type graphStepDedup struct {
	mu    sync.Mutex
	nodes map[string]struct{}
}

func newGraphStepDedup() *graphStepDedup {
	return &graphStepDedup{nodes: make(map[string]struct{})}
}

// graphNodeStartTracker records the first-observed node_start timestamp per
// node (2026-08-08 问题4b). First-write-wins: on node retry the earliest
// start is kept, so the persisted window spans all attempts — consistent with
// the member step-stream window (earliest StartedAt) used by
// MemberExecutionWindow.
type graphNodeStartTracker struct {
	mu    sync.Mutex
	nodes map[string]time.Time
}

func newGraphNodeStartTracker() *graphNodeStartTracker {
	return &graphNodeStartTracker{nodes: make(map[string]time.Time)}
}

func (t *graphNodeStartTracker) mark(nodeID string, at time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.nodes == nil {
		t.nodes = make(map[string]time.Time)
	}
	if _, exists := t.nodes[nodeID]; !exists {
		t.nodes[nodeID] = at
	}
}

func (t *graphNodeStartTracker) get(nodeID string) (time.Time, bool) {
	if t == nil {
		return time.Time{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	at, ok := t.nodes[nodeID]
	return at, ok
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
	if s.nodeStarts == nil {
		s.nodeStarts = newGraphNodeStartTracker()
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
		nodeStarts:      s.nodeStarts,
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
		nodeStarts:      newGraphNodeStartTracker(),
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

// MarkNodeStarted records the node_start notice timestamp (2026-08-08 问题4b).
// First-write-wins: node retry keeps the earliest start so the persisted step
// window spans all attempts. Nil-tracker safe.
func (c *GraphRunStepContext) MarkNodeStarted(nodeID string, at time.Time) {
	if c == nil || strings.TrimSpace(nodeID) == "" || at.IsZero() {
		return
	}
	c.nodeStarts.mark(nodeID, at)
}

// NodeStartedAt returns the tracked node_start timestamp, ok=false when the
// watch never observed node_start for this node (standalone / late-join watch).
func (c *GraphRunStepContext) NodeStartedAt(nodeID string) (time.Time, bool) {
	if c == nil {
		return time.Time{}, false
	}
	return c.nodeStarts.get(nodeID)
}
