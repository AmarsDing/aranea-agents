/**
 * Agent Tree Timeline types.
 *
 * Each Agent (root or child) is represented as an AgentBlock with a unified
 * internal structure: task → [timeline entries in chronological order] → result.
 * Root Agent blocks contain child Agent blocks in a tree layout.
 *
 * Timeline entries are ordered chronologically: thinking, tool, subagent
 * blocks appear in the order they occurred, not grouped by type.
 */

// ── Agent Block ──

export type AgentBlockStatus = 'running' | 'completed' | 'failed';

export interface AgentBlock {
  id: string;
  agentKey: string;
  agentName: string;
  agentIcon: string;
  /** Deterministic color derived from agentKey hash */
  agentColor: string;
  status: AgentBlockStatus;
  durationMs: number | null;
  collapsed: boolean;

  /** Task description received by this agent */
  task: string | null;
  /** Chronologically ordered timeline entries (thinking, tool, subagent) */
  timeline: TimelineEntry[];
  /** Execution result text */
  result: string | null;

  /** Orchestration plan (root agent only): task cards showing what will be done */
  plan: OrchestrationPlan | null;
  /** Team status summary (root agent only): aggregated from sub-agents */
  teamStatus: TeamStatusSummary | null;

  /** Timestamps */
  startedAt: string;
  finishedAt: string | null;
}

// ── Orchestration Plan (Trae-style task card) ──

export interface OrchestrationPlan {
  /** Plan entries — each represents a task to be assigned to a sub-agent */
  entries: PlanEntry[];
  /** Overall plan status */
  status: 'planning' | 'executing' | 'completed' | 'failed';
}

export interface PlanEntry {
  id: string;
  /** Task description */
  task: string;
  /** Assigned agent name (if known) */
  agentName: string | null;
  /** Agent icon */
  agentIcon: string | null;
  /** Agent color */
  agentColor: string | null;
  /** Entry status */
  status: 'pending' | 'running' | 'completed' | 'failed';
}

// ── Team Status Summary ──

export interface TeamStatusSummary {
  total: number;
  running: number;
  completed: number;
  failed: number;
}

// ── Timeline Entry (chronological) ──

/**
 * A reply is one of potentially multiple text-done blocks within a turn.
 * The model may emit a thinking → tool → partial reply → tool → final reply
 * sequence, and we want to preserve all reply chunks as distinct timeline
 * entries rather than collapsing them into a single `result` field.
 *
 * See docs/reports/2026-06-10-proposal-execution-progress-inline.md
 */
export interface ReplySection {
  id: string;
  content: string;
  durationMs: number | null;
  streaming: boolean;
}

/**
 * An orchestration progress step surfaced from a chat-visible
 * `execution_progress` envelope. Renders as an inline spinner / checkmark
 * card during the long 5-15s wait for LLM first byte.
 *
 * See docs/reports/2026-06-10-proposal-execution-progress-inline.md
 */
export type ProgressCategory = 'orchestration' | 'team' | 'tool' | 'thinking';

export const PROGRESS_CATEGORIES: readonly ProgressCategory[] = [
  'orchestration',
  'team',
  'tool',
  'thinking',
] as const;

/**
 * S8: Centralized glyph + label maps for the progress card UI.
 * Kept in `agentTreeTypes.ts` (the `*Ui.ts` equivalent for the chat domain)
 * so other surfaces (e.g. the plan panel, the team summary card) can reuse
 * the same iconography without copy-pasting emoji literals.
 *
 * If you need to add a new category, update both `PROGRESS_CATEGORIES` and
 * these two maps together — the union type will guide you.
 */
export const PROGRESS_GLYPHS: Readonly<Record<ProgressCategory, string>> = Object.freeze({
  orchestration: '⚙️',
  team: '👥',
  tool: '🛠️',
  thinking: '🧠',
});

export const PROGRESS_LABELS: Readonly<Record<ProgressCategory, string>> = Object.freeze({
  orchestration: '编排',
  team: '团队',
  tool: '工具',
  thinking: '思考',
});

/** Status icons used in the timeline card (S8 + T2). */
export const PROGRESS_STATUS_GLYPHS: Readonly<Record<'running' | 'done' | 'failed' | 'timeout', string>> =
  Object.freeze({
    running: '⏳',
    done: '✓',
    failed: '⚠️',
    timeout: '⚠️',
  });

export interface ProgressSection {
  /** step_id, e.g. "chat.llm.invoke" */
  id: string;
  category: ProgressCategory;
  /** User-facing Chinese short message, ≤ 24 chars */
  message: string;
  status: 'running' | 'done' | 'failed' | 'timeout';
  durationMs: number | null;
  /** Epoch ms when the step transitioned out of "running" (resolved_at). */
  startedAt: number;
}

export type TimelineEntry =
  | { kind: 'thinking'; section: ThinkingSection; sortKey: number }
  | { kind: 'tool'; section: ToolSection; sortKey: number }
  | { kind: 'reply'; section: ReplySection; sortKey: number }
  | { kind: 'progress'; section: ProgressSection; sortKey: number }
  | { kind: 'subagent'; block: AgentBlock; sortKey: number };

// ── Thinking Section ──

export interface ThinkingSection {
  id: string;
  content: string;
  durationMs: number;
  collapsed: boolean;
  /** Whether content is still streaming */
  streaming: boolean;
}

// ── Tool Section ──

export type ToolSectionStatus = 'running' | 'success' | 'failed' | 'blocked' | 'cancelled';

export interface ToolSection {
  id: string;
  toolName: string;
  toolLabel: string;
  status: ToolSectionStatus;
  durationMs: number | null;
  arguments: string | null;
  result: string | null;
  error: string | null;
  collapsed: boolean;
  /**
   * Optional icon hint from the upstream ToolUseEvent.icon_key field. When
   * present, AgentToolSection.vue uses it to pick a category glyph (file, web,
   * shell, MCP, etc.) instead of the generic ⚡ bolt.
   */
  iconKey?: string;
  /**
   * True when the orchestrator flagged this call as long-running (still alive
   * after the LLM kept streaming). Used by AgentToolSection.vue to surface a
   * "(等待中)" pill so users know the tool is intentionally silent.
   */
  isLongRunning?: boolean;
}

// ── Avatar color palette ──

/**
 * Returns a CSS color value for the given agent key.
 * Root agent uses --color-accent (gold in light mode, cyan in dark mode).
 * Sub-agents use --agent-palette-N CSS variables registered in theme files.
 */
const PALETTE_SIZE = 9;

export function agentColorFromKey(key: string): string {
  if (key === ROOT_AGENT_KEY) return 'var(--color-accent)';
  let hash = 0;
  for (let i = 0; i < key.length; i++) {
    hash = ((hash << 5) - hash + key.charCodeAt(i)) | 0;
  }
  return `var(--agent-palette-${Math.abs(hash) % PALETTE_SIZE})`;
}

/** Root agent key constant — the orchestrator spirit agent */
export const ROOT_AGENT_KEY = '__root__';
