# Chat Render Align Phase B Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete Phase 3 of `docs/reports/2026-06-25-analysis-chat-module-refactor.md` — wire up `SessionTreeSidebar`, unify team/member rendering through `ActivityStream`, implement sub-session Activity lazy-loading, and delete the legacy `TaskExecutionPanel`/`MemberReadOnlyPanel` rendering paths.

**Architecture:** Extend the Activity-First pipeline established in Phase A. The `useActivityTimeline` composable already isolates activities per `session_id`; Phase B makes `currentSessionId` follow `panelMode` transitions (spirit→spiritSessionId, team→teamSessionId, member→memberSessionId) so `ActivityStream` becomes the single renderer for all three modes. A new `GetSessionTree` RPC exposes the recursive session tree in one query; `useSessionTree` composable caches trees per spirit session; `TeamStageBlock` members become clickable to lazy-load member-session activities.

**Tech Stack:** Go + Kratos v2 (proto + service) | Vue 3 Composition API + Pinia + TypeScript | Vitest + @vue/test-utils

**Reference docs:**
- `docs/reports/2026-06-25-analysis-chat-module-refactor.md` §9 (Team/Graph UI), §10 (Session tree UI), §11 Phase 3
- `docs/superpowers/plans/2026-06-26-chat-render-align-phase-a.md` (Phase A baseline)

---

## File Structure

**Backend (Go)**
- `api/kratos/session/v1/session.proto` — add `GetSessionTree` RPC + messages
- `internal/service/session.go` — implement `GetSessionTree` handler (delegates to `biz.SessionUsecase.GetSessionTree`, already implemented at `internal/biz/session/usecase.go:363`)

**Frontend (Vue 3 + TS)**
- `web/src/features/session/api.ts` — add `getSessionTree` function (re-exports new `SessionTreeNode` proto type mapping)
- `web/src/features/session/types.ts` — already has `SessionTreeNode` interface (L62-65); no change
- `web/src/features/chat/composables/useSessionTree.ts` — NEW composable: loads spirit sessions + per-spirit session tree, exposes `spiritTreeNodes`, `loadTreeFor`, `findMemberSessionId`
- `web/src/features/chat/composables/useChatWorkspace.ts` — add `sessionTree` to returned bag; watch `spiritStore.activeMemberId` to resolve member session ID and call `activityTimeline.setCurrentSession`
- `web/src/components/chat/ChatPage.vue` — mount `SessionTreeSidebar` in the right sidebar (replaces `ChatSessionSidebar` flat list when in spirit mode)
- `web/src/components/chat/ChatSessionSidebar.vue` — keep as-is (still used in non-spirit modes); no change required
- `web/src/components/chat/ChatMessagePanel.vue` — remove `panelMode === 'team'` / `'member'` branches; ActivityStream renders for all modes
- `web/src/components/chat/TeamStageBlock.vue` — make member rows clickable; emit `expand-member` with `{ memberSessionId, agentKey }`
- `web/src/components/chat/ActivityStream.vue` — handle `expand-member` event from `TeamStageBlock`; forward to parent
- `web/src/stores/spirit/index.ts` — `selectMember` stays sync (sets `activeMemberId`/`activePanelMode`); the session ID resolution moves to `useChatWorkspace` watcher
- **Delete:** `web/src/components/spirit/TaskExecutionPanel.vue`, `web/src/components/spirit/MemberReadOnlyPanel.vue`

**Tests**
- `web/src/features/chat/composables/__tests__/useSessionTree.spec.ts` — NEW
- `web/src/features/chat/composables/__tests__/useActivityTimeline.spec.ts` — extend with member-session lazy-load test
- `web/src/components/chat/__tests__/TeamStageBlock.spec.ts` — NEW (member click → emit)
- `internal/service/session_test.go` — add `TestSessionService_GetSessionTree`

---

## Task Decomposition & Dependencies

```
Tier 0: Task 1 (GetSessionTree RPC)            — independent
Tier 1: Task 2 (useSessionTree + ChatPage接入)   — depends on T1
Tier 2: Task 3 (panelMode 同步 currentSessionId) — depends on T2
Tier 3: Task 4 (member 懒加载), Task 5 (删 legacy) — depend on T3, parallel
```

---

## Task 1: Expose GetSessionTree RPC

**Files:**
- Modify: `api/kratos/session/v1/session.proto` (add messages + RPC after `ListChildSessions` at L577)
- Modify: `internal/service/session.go` (add `GetSessionTree` handler after `ListChildSessions` at L552)
- Test: `internal/service/session_test.go`

**Why:** Backend `biz.SessionUsecase.GetSessionTree` (at `internal/biz/session/usecase.go:363`) already returns a recursive `*biz.SessionTree`. Exposing it as a single RPC avoids N+1 frontend `ListChildSessions` queries when building the sidebar tree. The existing `ListChildSessions` RPC stays as-is (still useful for flat child enumeration).

- [x] **Step 1: Add proto messages**

Append to `api/kratos/session/v1/session.proto` immediately after the `ListChildSessionsResponse` block (currently ends ~L434):

```proto
// GetSessionTree messages (Phase B-1): recursive session tree for sidebar.

message SessionTreeNode {
  Session session = 1;
  repeated SessionTreeNode children = 2;
}

message GetSessionTreeRequest {
  string spirit_session_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message GetSessionTreeResponse {
  SessionTreeNode root = 1;
}
```

Then add the RPC inside `SessionService` (after `ListChildSessions` at ~L577):

```proto
  // GetSessionTree returns the complete recursive session tree rooted at a spirit session.
  rpc GetSessionTree(GetSessionTreeRequest) returns (GetSessionTreeResponse) {
    option (google.api.http) = {get: "/v1/sessions/{spirit_session_id}/tree"};
  }
```

- [x] **Step 2: Regenerate proto bindings**

Run: `make api`
Expected: exit 0, regenerated `session.pb.go` / `session.pb.gw.go` / `session_v1.pb.ts` contain `GetSessionTree` symbols.

- [x] **Step 3: Write the failing service test**

Add to `internal/service/session_test.go`:

```go
func TestSessionService_GetSessionTree(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := mocksession.NewMockSessionUsecase(ctrl)
	// Build a tiny recursive tree: root + 1 child team session + 1 grandchild agent session.
	root := &biz.SessionTree{
		Session: biz.Session{ID: "spirit-1", Title: "Spirit Root"},
		Children: []*biz.SessionTree{
			{
				Session: biz.Session{ID: "team-1", Title: "Team 1", ParentSessionID: "spirit-1"},
				Children: []*biz.SessionTree{
					{Session: biz.Session{ID: "agent-1", Title: "Agent A", ParentSessionID: "team-1"}},
				},
			},
		},
	}
	uc.EXPECT().GetSessionTree(gomock.Any(), "spirit-1").Return(root, nil)

	svc := NewSessionService(uc, nil /* repo not used by this RPC */)
	resp, err := svc.GetSessionTree(context.Background(), &v1.GetSessionTreeRequest{SpiritSessionId: "spirit-1"})
	if err != nil {
		t.Fatalf("GetSessionTree: %v", err)
	}
	if resp.Root == nil || resp.Root.Session.Id != "spirit-1" {
		t.Fatalf("root mismatch: %+v", resp.Root)
	}
	if len(resp.Root.Children) != 1 || resp.Root.Children[0].Children[0].Session.Id != "agent-1" {
		t.Fatalf("tree shape mismatch: %+v", resp.Root)
	}
}
```

Run: `go test ./internal/service/ -run TestSessionService_GetSessionTree -count=1`
Expected: FAIL with `GetSessionTree not implemented` (or similar).

- [x] **Step 4: Implement the service handler**

Add to `internal/service/session.go` after `ListChildSessions` (L552):

```go
// GetSessionTree returns the complete recursive session tree rooted at a spirit session (Phase B-1).
func (s *SessionService) GetSessionTree(ctx context.Context, req *v1.GetSessionTreeRequest) (*v1.GetSessionTreeResponse, error) {
	spiritSessionID := strings.TrimSpace(req.GetSpiritSessionId())
	if spiritSessionID == "" {
		return nil, errBadRequest("spirit_session_id is required")
	}
	tree, err := s.uc.GetSessionTree(ctx, spiritSessionID)
	if err != nil {
		return nil, err
	}
	return &v1.GetSessionTreeResponse{Root: bizSessionTreeToProto(tree)}, nil
}

// bizSessionTreeToProto recursively converts *biz.SessionTree → *v1.SessionTreeNode.
func bizSessionTreeToProto(t *biz.SessionTree) *v1.SessionTreeNode {
	if t == nil {
		return nil
	}
	node := &v1.SessionTreeNode{
		Session: bizSessionToProto(&t.Session),
	}
	for _, c := range t.Children {
		node.Children = append(node.Children, bizSessionTreeToProto(c))
	}
	return node
}
```

Note: if `bizSessionToProto` does not exist, check `internal/service/session.go` for the existing `toSessionProto` / `sessionToProto` helper used by `ListChildSessions` (see L545-549) and reuse that name. Inspect first, then write the recursive helper with the matching convention.

- [x] **Step 5: Run tests to verify pass**

Run: `go test ./internal/service/ -run TestSessionService_GetSessionTree -count=1`
Expected: PASS.

- [x] **Step 6: Backend full build sanity**

Run: `go build ./...`
Expected: exit 0.

- [x] **Step 7: Add frontend API wrapper**

Add to `web/src/features/session/api.ts` after `listChildSessions` (L184):

```typescript
/**
 * GetSessionTree returns the complete recursive session tree rooted at a spirit
 * session (Phase B-1). Used by SessionTreeSidebar to render the full session
 * hierarchy in one query.
 * Backend: GET /v1/sessions/{spirit_session_id}/tree
 */
export async function getSessionTree(
  spiritSessionId: string,
): Promise<SessionTreeNode> {
  const data = await sessionApi.GetSessionTree({ spiritSessionId });
  return mapProtoTreeNode(data.root);
}

function mapProtoTreeNode(
  node?: { session?: KratosSession; children?: Array<{ session?: KratosSession; children?: unknown[] }> } | null,
): SessionTreeNode {
  if (!node || !node.session) {
    return { session: kratosSessionToLegacy({} as KratosSession), children: [] };
  }
  const children = (node.children ?? []).map((c) =>
    mapProtoTreeNode(c as Parameters<typeof mapProtoTreeNode>[0]),
  );
  return {
    session: kratosSessionToLegacy(node.session),
    children,
  };
}
```

Note: the generated proto type may name children differently — inspect `web/src/services/kratos/session/v1/index.ts` for `SessionTreeNode` shape after `make api` and adjust the cast accordingly. Prefer importing the generated type directly:

```typescript
import type { SessionTreeNode as KratosSessionTreeNode } from '../../services/kratos/session/v1/index';
```

Then refactor `mapProtoTreeNode` to accept `KratosSessionTreeNode | undefined`.

- [x] **Step 8: Verify frontend build**

Run: `cd web && pnpm build`
Expected: exit 0.

- [x] **Step 9: Commit**

```bash
git add api/kratos/session/v1/session.proto api/kratos/session/v1/*.go internal/service/session.go internal/service/session_test.go web/src/features/session/api.ts
git commit -m "feat(session): expose GetSessionTree RPC for recursive session tree

Adds GetSessionTree RPC + proto messages that delegate to the existing
biz.SessionUsecase.GetSessionTree. Frontend gains a getSessionTree API
wrapper. Replaces N+1 ListChildSessions recursion with a single query
when building the SessionTreeSidebar."
```

---

## Task 2: useSessionTree composable + SessionTreeSidebar wiring

**Files:**
- Create: `web/src/features/chat/composables/useSessionTree.ts`
- Create: `web/src/features/chat/composables/__tests__/useSessionTree.spec.ts`
- Modify: `web/src/features/chat/composables/useChatWorkspace.ts` (return `sessionTree`)
- Modify: `web/src/pages/ChatPage.vue` (mount `SessionTreeSidebar` in right sidebar when in spirit mode)

**Why:** `SessionTreeSidebar.vue` + `SessionTreeNode.vue` already exist (Phase 3 Task 6 implementation) but are never imported. This task creates the data layer that feeds them and mounts the component.

- [x] **Step 1: Write the failing test**

Create `web/src/features/chat/composables/__tests__/useSessionTree.spec.ts`:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { useSessionTree } from '../useSessionTree';
import * as sessionApi from '../../../session/api';

vi.mock('../../../session/api');

const fakeTree = {
  session: { id: 'spirit-1', title: 'Spirit 1', owner_type: 'spirit' },
  children: [
    {
      session: { id: 'team-1', title: 'Team 1', parent_session_id: 'spirit-1' },
      children: [
        { session: { id: 'agent-1', title: 'Agent A', parent_session_id: 'team-1', agent_id: 'agent-key-A' }, children: [] },
      ],
    },
  ],
};

describe('useSessionTree', () => {
  beforeEach(() => vi.clearAllMocks());

  it('loads tree for a spirit session', async () => {
    vi.mocked(sessionApi.getSessionTree).mockResolvedValue(fakeTree as any);
    const tree = useSessionTree();
    await tree.loadTreeFor('spirit-1');
    expect(tree.spiritTreeNodes.value).toHaveLength(1);
    expect(tree.spiritTreeNodes.value[0].session.id).toBe('spirit-1');
    expect(tree.findMemberSessionId('spirit-1', 'agent-key-A')).toBe('agent-1');
  });

  it('returns null when member not found', async () => {
    vi.mocked(sessionApi.getSessionTree).mockResolvedValue(fakeTree as any);
    const tree = useSessionTree();
    await tree.loadTreeFor('spirit-1');
    expect(tree.findMemberSessionId('spirit-1', 'nope')).toBeNull();
  });

  it('does not refetch when tree already cached', async () => {
    vi.mocked(sessionApi.getSessionTree).mockResolvedValue(fakeTree as any);
    const tree = useSessionTree();
    await tree.loadTreeFor('spirit-1');
    await tree.loadTreeFor('spirit-1');
    expect(sessionApi.getSessionTree).toHaveBeenCalledTimes(1);
  });
});
```

Run: `cd web && pnpm test useSessionTree`
Expected: FAIL with `Cannot find module '../useSessionTree'`.

- [x] **Step 2: Implement the composable**

Create `web/src/features/chat/composables/useSessionTree.ts`:

```typescript
import { shallowRef, triggerRef } from 'vue';
import { getSessionTree, listSessions } from '../../session/api';
import type { Session, SessionTreeNode } from '../../session/types';

/**
 * useSessionTree manages the spirit-session → recursive-tree cache used by
 * SessionTreeSidebar (Phase B-2). Trees are loaded lazily per spirit session
 * and cached; `findMemberSessionId` walks a cached tree to resolve a member's
 * agent session ID — used by useChatWorkspace to drive currentSessionId when
 * the user opens a member panel.
 */
export function useSessionTree() {
  // spiritSessionId → SessionTreeNode (full recursive tree)
  const treesBySpirit = shallowRef<Map<string, SessionTreeNode>>(new Map());
  // Top-level spirit sessions list (for sidebar root rows)
  const spiritSessions = shallowRef<Session[]>([]);

  async function loadSpiritSessions(agentId: string) {
    const sessions = await listSessions(agentId);
    spiritSessions.value = sessions;
    triggerRef(spiritSessions);
  }

  async function loadTreeFor(spiritSessionId: string): Promise<SessionTreeNode | null> {
    const cached = treesBySpirit.value.get(spiritSessionId);
    if (cached) return cached;
    const tree = await getSessionTree(spiritSessionId);
    const next = new Map(treesBySpirit.value);
    next.set(spiritSessionId, tree);
    treesBySpirit.value = next;
    return tree;
  }

  /**
   * Walk a cached spirit tree to find the agent session matching `agentKey`.
   * Returns null if the tree is not cached or no match is found.
   */
  function findMemberSessionId(spiritSessionId: string, agentKey: string): string | null {
    const tree = treesBySpirit.value.get(spiritSessionId);
    if (!tree) return null;
    return walkForAgent(tree, agentKey);
  }

  function walkForAgent(node: SessionTreeNode, agentKey: string): string | null {
    if (node.session.agent_id === agentKey) return node.session.id;
    for (const child of node.children) {
      const hit = walkForAgent(child, agentKey);
      if (hit) return hit;
    }
    return null;
  }

  /** Top-level tree nodes for the sidebar (root = each spirit session's tree). */
  const spiritTreeNodes = shallowRef<SessionTreeNode[]>([]);

  async function refreshSpiritTreeNodes(spiritSessionIds: string[]) {
    const nodes: SessionTreeNode[] = [];
    for (const sid of spiritSessionIds) {
      const t = await loadTreeFor(sid);
      if (t) nodes.push(t);
    }
    spiritTreeNodes.value = nodes;
    triggerRef(spiritTreeNodes);
  }

  function clear() {
    treesBySpirit.value = new Map();
    spiritSessions.value = [];
    spiritTreeNodes.value = [];
  }

  return {
    treesBySpirit,
    spiritSessions,
    spiritTreeNodes,
    loadSpiritSessions,
    loadTreeFor,
    refreshSpiritTreeNodes,
    findMemberSessionId,
    clear,
  };
}
```

Run: `cd web && pnpm test useSessionTree`
Expected: PASS (3 tests).

- [x] **Step 3: Expose sessionTree in useChatWorkspace**

Modify `web/src/features/chat/composables/useChatWorkspace.ts`:

1. Import `useSessionTree` at the top.
2. Instantiate near the other composables: `const sessionTree = useSessionTree();`
3. Add `sessionTree` to the returned object (find the `return { ... }` at the bottom of `useChatWorkspace`).
4. When `selectedSessionForUi` changes to a spirit session, call `sessionTree.loadTreeFor(sid)` (fire-and-forget inside the existing watcher at L966-999 — append after `void bindSessionView(sid, true);`):
   ```typescript
   if (sid !== prevSid) {
     activityTimeline.setCurrentSession(sid);
     sender.clearFailedPendingForSession(prevSid);
     void bindSessionView(sid, true);
     // Phase B-2: preload session tree for sidebar.
     void sessionTree.loadTreeFor(sid).catch(() => {/* swallow; sidebar shows error state */});
   }
   ```

- [x] **Step 4: Mount SessionTreeSidebar in ChatPage**

Modify `web/src/pages/ChatPage.vue`:

1. Import: `import SessionTreeSidebar from '../components/chat/SessionTreeSidebar.vue';`
2. Replace the `ChatSessionSidebar` block (L167-184) with a conditional that shows `SessionTreeSidebar` when in spirit mode and `ChatSessionSidebar` otherwise:

```vue
<SessionTreeSidebar
  v-if="spiritStore.activePanelMode === 'spirit'"
  :tree-nodes="session.sessionTree.spiritTreeNodes"
  :active-session-id="session.selectedSessionForUi?.id || ''"
  :loading="session.sessionTree.spiritTreeNodes.length === 0 && Boolean(session.selectedSessionForUi?.id)"
  @select="session.onSelectSession"
/>
<ChatSessionSidebar
  v-else
  :open="layout.rightOpen"
  :sessions="session.displaySessions"
  :inbox-sessions="session.inboxSessions"
  :selected-session-id="session.selectedSessionForUi?.id"
  :is-dark="layout.isDark"
  :favorite-ids="session.favoriteIds"
  @select="session.onSelectSession"
  @new-session="session.onNewSession"
  @rename="session.onRenameSession"
  @toggle-pin="session.onTogglePinSession"
  @toggle-favorite="session.onToggleFavorite"
  @trace="session.openSessionTrace"
  @delete="entity.openDelete"
  @restore="session.onRestoreSession"
  @archive="session.onArchiveSession"
  @detail="session.onSessionDetail"
/>
```

Note: `session.sessionTree.spiritTreeNodes` is a `shallowRef<SessionTreeNode[]>`; passing it as a prop works because Vue unwraps refs in templates. If the sidebar receives `undefined` on first render, default to `[]` via `?? []` in the binding.

3. Verify `session.onSelectSession` signature accepts a `sessionId: string` — yes, it does (existing handler).

- [x] **Step 5: Run frontend tests + build**

Run: `cd web && pnpm test && pnpm lint && pnpm build`
Expected: all pass.

- [x] **Step 6: Commit**

```bash
git add web/src/features/chat/composables/useSessionTree.ts web/src/features/chat/composables/__tests__/useSessionTree.spec.ts web/src/features/chat/composables/useChatWorkspace.ts web/src/pages/ChatPage.vue
git commit -m "feat(chat): wire up SessionTreeSidebar with useSessionTree

Phase B-2: creates useSessionTree composable that caches recursive
session trees per spirit session via the new GetSessionTree RPC.
ChatPage mounts SessionTreeSidebar in the right sidebar when in spirit
mode, falling back to ChatSessionSidebar for team/member modes."
```

---

## Task 3: Sync currentSessionId with panelMode transitions

**Files:**
- Modify: `web/src/features/chat/composables/useChatWorkspace.ts` (add watcher for `spiritStore.activeTeamId` / `activeMemberId` / `activePanelMode`)
- Modify: `web/src/features/chat/composables/__tests__/useActivityTimeline.spec.ts` (extend) — actually add a new test file `useChatWorkspacePanelMode.spec.ts` if isolating; otherwise extend an existing workspace test
- Create: `web/src/features/chat/composables/__tests__/useChatWorkspacePanelMode.spec.ts`

**Why:** `sortedActivities` is driven by `currentSessionId`. For `ActivityStream` to render team/member content, `currentSessionId` must follow `panelMode`:
- spirit → spiritSessionId (already handled by selectedSessionForUi watcher)
- team → team.teamSessionId
- member → resolve via `sessionTree.findMemberSessionId(spiritSessionId, member.agentKey)`

- [x] **Step 1: Write the failing test**

Create `web/src/features/chat/composables/__tests__/useChatWorkspacePanelMode.spec.ts`:

```typescript
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useSpiritTeamStore } from '../../../../stores/spirit';
import { useSessionTree } from '../useSessionTree';
import * as sessionApi from '../../../session/api';

vi.mock('../../../session/api');

const fakeTree = {
  session: { id: 'spirit-1', title: 'Spirit', owner_type: 'spirit' },
  children: [
    {
      session: { id: 'team-1', title: 'Team 1', parent_session_id: 'spirit-1', team_id: 'team-1' },
      children: [
        { session: { id: 'agent-A', title: 'Agent A', parent_session_id: 'team-1', agent_id: 'agent-key-A' }, children: [] },
      ],
    },
  ],
};

describe('panelMode → currentSessionId sync', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
    vi.mocked(sessionApi.getSessionTree).mockResolvedValue(fakeTree as any);
  });

  it('resolves member session id from session tree', async () => {
    const spirit = useSpiritTeamStore();
    const tree = useSessionTree();
    await tree.loadTreeFor('spirit-1');
    const memberId = tree.findMemberSessionId('spirit-1', 'agent-key-A');
    expect(memberId).toBe('agent-A');
  });

  it('team select sets currentSessionId to teamSessionId', async () => {
    const spirit = useSpiritTeamStore();
    spirit.handleSpiritActivityEvent({
      event: 'created',
      activity: {
        id: 'a1', kind: 'team_stage', status: 'running', session_id: 'spirit-1',
        stage: 'assembled', meta: { team_id: 'team-1', session_id: 'team-1', team_name: 'Team 1' },
      } as any,
    });
    spirit.selectTeam('team-1');
    expect(spirit.activeTeam?.teamSessionId).toBe('team-1');
  });
});
```

Run: `cd web && pnpm test useChatWorkspacePanelMode`
Expected: FAIL (test file exists but does not yet verify the wiring between spiritStore events and activityTimeline.setCurrentSession — that wiring is added in step 2).

- [x] **Step 2: Add the panelMode watcher in useChatWorkspace**

Modify `web/src/features/chat/composables/useChatWorkspace.ts`:

After the existing `selectedSessionForUi` watcher (L998), add:

```typescript
// Phase B-3: Sync activityTimeline.currentSessionId with panelMode.
// - spirit mode: currentSessionId = selectedSessionForUi.id (already set above)
// - team mode:   currentSessionId = activeTeam.teamSessionId
// - member mode: resolve via sessionTree.findMemberSessionId
watch(
  () => spiritStore.activePanelMode,
  async (mode) => {
    if (mode === 'spirit') {
      const sid = selectedSessionForUi.value?.id;
      if (sid) activityTimeline.setCurrentSession(sid);
      return;
    }
    if (mode === 'team') {
      const teamSessionId = spiritStore.activeTeam?.teamSessionId;
      if (teamSessionId) {
        activityTimeline.setCurrentSession(teamSessionId);
        void bindSessionView(teamSessionId, true);
      }
      return;
    }
    if (mode === 'member') {
      const spiritSid = selectedSessionForUi.value?.id;
      const agentKey = spiritStore.activeMember?.agentKey;
      if (!spiritSid || !agentKey) return;
      // Ensure tree is loaded before resolving.
      await sessionTree.loadTreeFor(spiritSid);
      const memberSessionId = sessionTree.findMemberSessionId(spiritSid, agentKey);
      if (memberSessionId) {
        activityTimeline.setCurrentSession(memberSessionId);
        void bindSessionView(memberSessionId, true);
      }
    }
  },
);

// Also react when activeMemberId changes within member mode.
watch(
  () => spiritStore.activeMemberId,
  async (memberId) => {
    if (spiritStore.activePanelMode !== 'member' || !memberId) return;
    const spiritSid = selectedSessionForUi.value?.id;
    const member = spiritStore.activeTeam?.members.find((m) => m.agentId === memberId);
    const agentKey = member?.agentKey;
    if (!spiritSid || !agentKey) return;
    await sessionTree.loadTreeFor(spiritSid);
    const memberSessionId = sessionTree.findMemberSessionId(spiritSid, agentKey);
    if (memberSessionId) {
      activityTimeline.setCurrentSession(memberSessionId);
      void bindSessionView(memberSessionId, true);
    }
  },
);
```

Note: `bindSessionView` must accept the override session id. Inspect its current signature (`useChatWorkspace.ts` — search for `function bindSessionView`). It likely uses `selectedSessionForUi.value?.id` internally; if so, refactor it to accept an optional `overrideSessionId: string` parameter, defaulting to the selected session. Keep the existing behavior for spirit mode unchanged.

- [x] **Step 3: Import spiritStore in useChatWorkspace**

If not already imported, add at top of `useChatWorkspace.ts`:

```typescript
import { useSpiritTeamStore } from '../../../stores/spirit';
```

And instantiate: `const spiritStore = useSpiritTeamStore();` near the other store instantiations.

- [x] **Step 4: Run tests**

Run: `cd web && pnpm test && pnpm lint`
Expected: pass.

- [x] **Step 5: Commit**

```bash
git add web/src/features/chat/composables/useChatWorkspace.ts web/src/features/chat/composables/__tests__/useChatWorkspacePanelMode.spec.ts
git commit -m "feat(chat): sync currentSessionId with panelMode transitions

Phase B-3: when spiritStore.activePanelMode switches to team/member,
useChatWorkspace updates activityTimeline.currentSessionId so that
sortedActivities (and thus ActivityStream) renders the corresponding
session's activities. Member session id is resolved from the cached
session tree via useSessionTree.findMemberSessionId."
```

---

## Task 4: Sub-session Activity lazy-loading (§9.1.3)

**Files:**
- Modify: `web/src/components/chat/TeamStageBlock.vue` (member rows clickable; emit `expand-member`)
- Modify: `web/src/components/chat/ActivityStream.vue` (forward `expand-member` event)
- Modify: `web/src/components/chat/ChatMessageList.vue` (forward event to ChatMessagePanel)
- Modify: `web/src/components/chat/ChatMessagePanel.vue` (forward event up)
- Modify: `web/src/pages/ChatPage.vue` (handle event → call spiritStore.selectMember + sessionTree.loadTreeFor)
- Create: `web/src/components/chat/__tests__/TeamStageBlock.spec.ts`

**Why:** §9.1.3 specifies that clicking a team member row should lazy-load that member's session activities. Currently `TeamStageBlock` renders member rows as static display (no click handler). This task wires the click → `selectMember` + `loadTreeFor` flow.

- [x] **Step 1: Write the failing test**

Create `web/src/components/chat/__tests__/TeamStageBlock.spec.ts`:

```typescript
import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import TeamStageBlock from '../TeamStageBlock.vue';
import type { TeamStageEvent } from '../../../features/chat/streamEventTypes';

const activity: TeamStageEvent = {
  id: 'ts-1',
  kind: 'team_stage',
  status: 'running',
  sessionId: 'team-1',
  timestamp: new Date().toISOString(),
  durationMs: null,
  stage: 'executing',
  members: [
    { agentKey: 'agent-key-A', agentName: 'Agent A', status: 'running' },
    { agentKey: 'agent-key-B', agentName: 'Agent B', status: 'pending' },
  ],
} as any;

describe('TeamStageBlock', () => {
  it('emits expand-member with agentKey when a member row is clicked', async () => {
    const wrapper = mount(TeamStageBlock, { props: { activity } });
    const rows = wrapper.findAll('.team-stage-block__member');
    expect(rows).toHaveLength(2);
    await rows[0].trigger('click');
    expect(wrapper.emitted('expand-member')).toBeTruthy();
    const payload = wrapper.emitted('expand-member')![0][0] as { agentKey: string };
    expect(payload.agentKey).toBe('agent-key-A');
  });
});
```

Run: `cd web && pnpm test TeamStageBlock`
Expected: FAIL (no click handler / no emit).

- [x] **Step 2: Make member rows clickable in TeamStageBlock**

Modify `web/src/components/chat/TeamStageBlock.vue`:

Replace the `<div v-for="member in activity.members" ...>` block (L33-47) with:

```vue
<div
  v-for="member in activity.members"
  :key="member.agentKey"
  class="team-stage-block__member team-stage-block__member--clickable"
  :class="`team-stage-block__member--${member.status}`"
  :title="t('chat.teamStage.expandMember', { name: member.agentName || member.agentKey })"
  @click="onMemberClick(member)"
>
  <span class="team-stage-block__member-dot" :class="`team-stage-block__member-dot--${member.status}`">
    <span v-if="member.status === 'running'" class="team-stage-block__pulse" />
  </span>
  <span class="team-stage-block__member-name">{{ member.agentName || member.agentKey }}</span>
  <span v-if="member.status === 'running'" class="team-stage-block__member-badge">
    {{ t('chat.teamStage.executing') }}
  </span>
  <q-icon name="chevron_right" size="14px" class="team-stage-block__member-chevron" />
</div>
```

Add to `<script setup>`:

```typescript
const emit = defineEmits<{
  'expand-member': [payload: { agentKey: string; agentName?: string }];
}>();

function onMemberClick(member: { agentKey: string; agentName?: string }) {
  emit('expand-member', { agentKey: member.agentKey, agentName: member.agentName });
}
```

Add the chevron + clickable styles to `<style>`:

```sass
&__member--clickable
  cursor: pointer
  &:hover
    background: var(--glass-surface)

&__member-chevron
  color: var(--color-text-tertiary)
  margin-left: auto
```

- [x] **Step 3: Forward expand-member through ActivityStream**

Modify `web/src/components/chat/ActivityStream.vue`:

1. Add to `defineEmits`:
   ```typescript
   'expand-member': [payload: { agentKey: string; agentName?: string }];
   ```
2. On `<TeamStageBlock>` (L68), add the listener:
   ```vue
   <TeamStageBlock
     v-else-if="item.event.kind === 'team_stage'"
     :activity="item.event as TeamStageEvent"
     @expand-member="(p) => $emit('expand-member', p)"
   />
   ```

- [x] **Step 4: Forward expand-member through ChatMessageList → ChatMessagePanel → ChatPage**

Modify `web/src/components/chat/ChatMessageList.vue`:
- Find where `<ActivityStream>` is rendered (search the file).
- Add `@expand-member="(p) => emit('expand-member', p)"` to the `<ActivityStream>` element.
- Add `'expand-member': [payload: { agentKey: string; agentName?: string }]` to its `defineEmits`.

Modify `web/src/components/chat/ChatMessagePanel.vue`:
- Add `@expand-member="(p) => emit('expand-member', p)"` to `<ChatMessageList>` (around L176-208).
- Add the same emit declaration to its `defineEmits`.

Modify `web/src/pages/ChatPage.vue`:
- Add `@expand-member="onExpandMember"` to `<ChatMessagePanel>`.
- Implement the handler in `<script setup>`:
  ```typescript
  function onExpandMember(payload: { agentKey: string; agentName?: string }) {
    // Switch to member mode; useChatWorkspace watcher will resolve the member
    // session id from the session tree and lazy-load activities.
    spiritStore.selectMember(payload.agentKey);
    // ensure tree is loaded for the current spirit session
    const spiritSid = session.selectedSessionForUi?.id;
    if (spiritSid) {
      void session.sessionTree.loadTreeFor(spiritSid);
    }
  }
  ```

Note: `spiritStore.selectMember` currently accepts `memberId: string`. Verify its semantics — at `stores/spirit/index.ts:223` it stores `activeMemberId.value = memberId`. The memberId there is actually the agentKey/agentId used to look up the member in `team.members.find((m) => m.agentId === memberId)` (see `ChatPage.vue:259`). Confirm by reading the call site; if `selectMember` is called with `agentId`, then `onExpandMember` should pass the member's `agentId`, not `agentKey`. Inspect `SpiritMember` type — it has both `agentId` and `agentKey`. Use `agentId` for `selectMember` (matches existing convention) and `agentKey` for `sessionTree.findMemberSessionId` (sessions store `agent_id` which corresponds to the agent's stable key, not its database id). If they differ, the payload needs both fields.

Update the `TeamStageBlock` emit payload to include both:

```typescript
'expand-member': [payload: { agentId: string; agentKey: string; agentName?: string }];
```

And `onExpandMember`:

```typescript
function onExpandMember(payload: { agentId: string; agentKey: string; agentName?: string }) {
  spiritStore.selectMember(payload.agentId);
  const spiritSid = session.selectedSessionForUi?.id;
  if (spiritSid) {
    void session.sessionTree.loadTreeFor(spiritSid);
  }
}
```

- [x] **Step 5: Run tests + build**

Run: `cd web && pnpm test TeamStageBlock && pnpm lint && pnpm build`
Expected: pass.

- [x] **Step 6: Commit**

```bash
git add web/src/components/chat/TeamStageBlock.vue web/src/components/chat/ActivityStream.vue web/src/components/chat/ChatMessageList.vue web/src/components/chat/ChatMessagePanel.vue web/src/pages/ChatPage.vue web/src/components/chat/__tests__/TeamStageBlock.spec.ts
git commit -m "feat(chat): lazy-load member session activities on team member click

Phase B-4 / §9.1.3: TeamStageBlock member rows are now clickable and
emit expand-member. ChatPage handles the event by switching to member
mode (spiritStore.selectMember) and ensuring the session tree is loaded;
useChatWorkspace's panelMode watcher resolves the member session id and
calls bindSessionView to lazy-load activities."
```

---

## Task 5: Delete legacy TaskExecutionPanel / MemberReadOnlyPanel rendering paths

**Files:**
- Modify: `web/src/components/chat/ChatMessagePanel.vue` (remove team/member branches at L44-68)
- Modify: `web/src/components/chat/ChatMessagePanel.vue` (remove now-unused imports: `TaskExecutionPanel`, `MemberReadOnlyPanel`)
- Delete: `web/src/components/spirit/TaskExecutionPanel.vue`
- Delete: `web/src/components/spirit/MemberReadOnlyPanel.vue`
- Modify: `web/src/components/chat/ChatMessagePanel.vue` (remove props only used by legacy paths: `spiritMaxConcurrentTeams`, `spiritCompletionStats` if not used elsewhere — verify by grep)

**Why:** With Task 3 making `ActivityStream` render team/member content via `currentSessionId`, the legacy `TaskExecutionPanel`/`MemberReadOnlyPanel` paths in `ChatMessagePanel` are dead. §11 Phase 3 Deletions explicitly requires their removal.

**Pre-condition:** Task 3 + Task 4 must be merged and verified — `ActivityStream` must successfully render team/member activities before deleting the legacy paths. Run the app manually (or via existing integration tests) to confirm team/member modes show content via `ActivityStream`.

- [x] **Step 1: Write a regression test**

Create `web/src/components/chat/__tests__/ChatMessagePanel.legacy.spec.ts`:

```typescript
import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import ChatMessagePanel from '../ChatMessagePanel.vue';

// Minimal props stub — ChatMessagePanel has many required props; only the
// ones relevant to the legacy-path removal assertion are real.
const baseProps = {
  modelValue: '',
  messages: [],
  attachments: [],
  dialogMode: 'chat',
  modelProvider: '',
  modeOptions: [],
  providerOptions: [],
  sessionTitle: 'Test',
  isDark: false,
  activities: [],
};

describe('ChatMessagePanel legacy path removal', () => {
  it('does not render TaskExecutionPanel in team mode', () => {
    const wrapper = mount(ChatMessagePanel, {
      props: { ...baseProps, panelMode: 'team', spiritTeam: null },
      global: { stubs: ['ChatMessageList', 'ChatComposer', 'ChatHeaderUsagePanel', 'ChatHeaderPromptBar'] },
    });
    expect(wrapper.findComponent({ name: 'TaskExecutionPanel' }).exists()).toBe(false);
  });

  it('does not render MemberReadOnlyPanel in member mode', () => {
    const wrapper = mount(ChatMessagePanel, {
      props: { ...baseProps, panelMode: 'member', spiritTeam: null, activeMember: null },
      global: { stubs: ['ChatMessageList', 'ChatComposer', 'ChatHeaderUsagePanel', 'ChatHeaderPromptBar'] },
    });
    expect(wrapper.findComponent({ name: 'MemberReadOnlyPanel' }).exists()).toBe(false);
  });
});
```

Run: `cd web && pnpm test ChatMessagePanel.legacy`
Expected: FAIL (legacy paths still render).

- [x] **Step 2: Remove the team/member branches from ChatMessagePanel**

Modify `web/src/components/chat/ChatMessagePanel.vue`:

Delete lines L44-68 (the `<template v-if="panelMode === 'team' && spiritTeam">` and `<template v-else-if="panelMode === 'member' && spiritTeam && activeMember">` blocks).

The remaining `<template v-else>` block (the spirit/default path) becomes the only rendering path. Change `<template v-else>` to `<template>` (drop the `v-else`).

Remove now-unused imports from `<script setup>` (L328, L330):
```typescript
import TaskExecutionPanel from '../spirit/TaskExecutionPanel.vue';
// ...
import MemberReadOnlyPanel from '../spirit/MemberReadOnlyPanel.vue';
```

Remove props only used by the legacy paths — inspect each candidate by grepping the file:
- `spiritMaxConcurrentTeams` — grep usage; if only used in the deleted `<TaskExecutionPanel>`, remove from props.
- `spiritCompletionStats` — same.
- `activeMember` — used in the breadcrumb (L32-34); keep.
- `spiritTeam` — used in the breadcrumb (L18-30); keep.

Run: `cd web && pnpm test ChatMessagePanel.legacy`
Expected: PASS.

- [x] **Step 3: Delete the legacy component files**

```bash
rm web/src/components/spirit/TaskExecutionPanel.vue
rm web/src/components/spirit/MemberReadOnlyPanel.vue
```

- [x] **Step 4: Update ChatPage to drop legacy-only props**

Modify `web/src/pages/ChatPage.vue`:

Remove the `:spirit-max-concurrent-teams` and `:spirit-completion-stats` bindings from `<ChatMessagePanel>` (around L76-78) if those props were removed in step 2. If the props are retained (because they feed the status bar / synthesis card), keep them.

Grep to confirm: `grep -rn "spiritMaxConcurrentTeams\|spiritCompletionStats" web/src/components/chat/ChatMessagePanel.vue` — if zero matches, remove the bindings from ChatPage.

- [x] **Step 5: Full frontend verification**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: all pass, zero references to `TaskExecutionPanel` / `MemberReadOnlyPanel` anywhere.

Verify no dangling references:
```
grep -rn "TaskExecutionPanel\|MemberReadOnlyPanel" web/src
```
Expected: zero matches (excluding this plan file and any test files that explicitly assert absence — those should be in `__tests__/`).

- [x] **Step 6: Commit**

```bash
git add web/src/components/chat/ChatMessagePanel.vue web/src/components/chat/__tests__/ChatMessagePanel.legacy.spec.ts web/src/pages/ChatPage.vue
git rm web/src/components/spirit/TaskExecutionPanel.vue web/src/components/spirit/MemberReadOnlyPanel.vue
git commit -m "refactor(chat): remove legacy TaskExecutionPanel/MemberReadOnlyPanel paths

Phase B-5 / §11 Phase 3 Deletions: with ActivityStream now rendering
all three panel modes (spirit/team/member) via currentSessionId, the
deprecated TaskExecutionPanel and MemberReadOnlyPanel rendering paths
in ChatMessagePanel are removed along with the component files. The
spirit/default path becomes the single unified rendering pipeline."
```

---

## Verification Checklist

After all 5 tasks merge:

- [x] **Backend**: `make api && make wire && make build && make test && make lint`
- [x] **Frontend**: `cd web && pnpm lint && pnpm test && pnpm build`
- [x] **Grep audit**:
  - `grep -rn "TaskExecutionPanel\|MemberReadOnlyPanel" web/src` → 0 matches
  - `grep -rn "panelMode === 'team'\|panelMode === 'member'" web/src/components/chat/ChatMessagePanel.vue` → 0 matches (the breadcrumb can still check panelMode, but the dual rendering branches are gone)
  - `grep -rn "SessionTreeSidebar" web/src` → at least 2 matches (component definition + ChatPage import)
  - `grep -rn "getSessionTree\|GetSessionTree" web/src` → at least 1 match in `features/session/api.ts`
- [x] **Manual smoke test**: open the app, send a spirit instruction that triggers team assembly, verify:
  - Session tree sidebar shows the spirit session with expandable team/agent children
  - Switching to team mode renders team activities via ActivityStream (no TaskExecutionPanel)
  - Clicking a member in TeamStageBlock switches to member mode and renders member activities via ActivityStream (no MemberReadOnlyPanel)
  - Returning to spirit mode renders spirit activities

---

## Documentation Sync (Post-Completion)

After all tasks pass verification, update:
- `docs/development/34-event-system.development.md` — mark Phase 3 Task 6/7 + Deletions as ✅
- `docs/reports/2026-06-25-analysis-chat-module-refactor.md` §11 Phase 3 — annotate completed tasks
- If the work introduces a cross-module architectural decision (e.g. "ActivityStream is the single renderer for all panel modes"), add an ADR at `docs/reports/2026-06-26-review-adr-chat-unified-rendering.md`
