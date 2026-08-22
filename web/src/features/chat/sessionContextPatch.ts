import type { ActivityEvent } from '../../realtime/activityEvent';
import type { Session } from '../session/types';
import { parseContextBudgetMeta } from './contextBudget';
import { CHAT_CONTEXT_WINDOW_TOKENS, chatContextRatio, contextStatusFromRatio } from '../session/contextMetrics';

/** Token-usage payload derived from an ActivityEvent's meta fields. */
export type ActivityUsage = {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  max_tokens?: number;
  /** Max prompt tokens in the turn — context window fill (ReAct uses peak prompt). */
  context_prompt_tokens?: number;
  turn_total_tokens?: number;
};

export type SessionContextPatch = Partial<
  Pick<
    Session,
    | 'context_used_ratio'
    | 'context_used_tokens'
    | 'context_status'
    | 'total_tokens'
    | 'max_context_used_ratio'
    | 'input_tokens'
    | 'output_tokens'
    | 'total_cost_micro_usd'
    | 'last_context_window_tokens'
    | 'message_count'
    | 'model_call_count'
    | 'tool_call_count'
    | 'skill_call_count'
    | 'mcp_call_count'
    | 'context_budget'
  >
>;

/** Prompt tokens used for context-window fill (max in turn), distinct from billing aggregates. */
export function contextPromptTokensFromUsage(usage: ActivityUsage | undefined): number {
  if (!usage) return 0;
  const explicit = usage.context_prompt_tokens ?? 0;
  if (explicit > 0) return explicit;
  return usage.prompt_tokens ?? 0;
}

export function contextRatioFromUsage(usage: ActivityUsage | undefined): number | null {
  const prompt = contextPromptTokensFromUsage(usage);
  return chatContextRatio(prompt);
}

export { contextStatusFromRatio } from '../session/contextMetrics';

/** Mid-turn ReAct sub-step: update context bar only (no session total_tokens increment). */
export function sessionContextPatchFromStepUsage(usage: ActivityUsage | undefined): SessionContextPatch | null {
  const ratio = contextRatioFromUsage(usage);
  if (ratio == null || !usage) return null;

  const contextPrompt = contextPromptTokensFromUsage(usage);
  const patch: SessionContextPatch = {
    context_used_ratio: ratio,
    context_used_tokens: contextPrompt,
    context_status: contextStatusFromRatio(ratio),
  };

  patch.last_context_window_tokens = CHAT_CONTEXT_WINDOW_TOKENS;

  return patch;
}

export function sessionContextPatchFromUsage(
  usage: ActivityUsage | undefined,
  prev?: Pick<Session, 'total_tokens' | 'max_context_used_ratio' | 'input_tokens' | 'output_tokens'>,
): SessionContextPatch | null {
  const ratio = contextRatioFromUsage(usage);
  if (ratio == null || !usage) return null;

  const contextPrompt = contextPromptTokensFromUsage(usage);
  const patch: SessionContextPatch = {
    context_used_ratio: ratio,
    context_used_tokens: contextPrompt,
    context_status: contextStatusFromRatio(ratio),
  };

  patch.last_context_window_tokens = CHAT_CONTEXT_WINDOW_TOKENS;

  const turnTok = usage.turn_total_tokens ?? usage.total_tokens ?? 0;
  const prompt = usage.prompt_tokens ?? 0;
  const completion = usage.completion_tokens ?? 0;
  if (turnTok > 0) {
    patch.total_tokens = (prev?.total_tokens ?? 0) + turnTok;
  }
  if (prompt > 0) {
    patch.input_tokens = (prev?.input_tokens ?? 0) + prompt;
  }
  if (completion > 0) {
    patch.output_tokens = (prev?.output_tokens ?? 0) + completion;
  }

  const prevMax = prev?.max_context_used_ratio ?? 0;
  if (ratio > prevMax) {
    patch.max_context_used_ratio = ratio;
  }

  return patch;
}

export function sessionContextPatchFromCompressMeta(
  meta: Record<string, unknown> | undefined,
): SessionContextPatch | null {
  if (!meta) return null;
  const ratio = typeof meta.context_used_ratio === 'number' ? meta.context_used_ratio : null;
  if (ratio == null) return null;

  const patch: SessionContextPatch = {
    context_used_ratio: ratio,
    context_status: typeof meta.context_status === 'string' ? meta.context_status : contextStatusFromRatio(ratio),
  };

  if (typeof meta.context_used_tokens === 'number') {
    patch.context_used_tokens = meta.context_used_tokens;
  }
  patch.last_context_window_tokens = CHAT_CONTEXT_WINDOW_TOKENS;

  return patch;
}

export function reconcilePatchFromServer(
  server: Session,
  local?: Pick<
    Session,
    | 'total_tokens'
    | 'max_context_used_ratio'
    | 'input_tokens'
    | 'output_tokens'
    | 'total_cost_micro_usd'
    | 'message_count'
    | 'model_call_count'
    | 'tool_call_count'
    | 'skill_call_count'
    | 'mcp_call_count'
    | 'context_used_ratio'
    | 'context_used_tokens'
  >,
): SessionContextPatch {
  // For context metrics: use Math.max to prevent stale server values from
  // overwriting locally-accumulated real-time WS values. During an active turn,
  // the frontend receives context_usage events that are more recent than the
  // server's last flush. After compression, the flush includes the compression
  // delta so the server value is post-compression; Math.max still works because
  // a new turn's context_usage will have a higher ratio.
  const reconciledRatio = Math.max(server.context_used_ratio ?? 0, local?.context_used_ratio ?? 0);
  return {
    context_used_ratio: reconciledRatio,
    context_used_tokens: Math.max(server.context_used_tokens ?? 0, local?.context_used_tokens ?? 0),
    context_status: contextStatusFromRatio(reconciledRatio),
    total_tokens: Math.max(server.total_tokens ?? 0, local?.total_tokens ?? 0),
    max_context_used_ratio: Math.max(server.max_context_used_ratio ?? 0, local?.max_context_used_ratio ?? 0),
    input_tokens: Math.max(server.input_tokens ?? 0, local?.input_tokens ?? 0),
    output_tokens: Math.max(server.output_tokens ?? 0, local?.output_tokens ?? 0),
    total_cost_micro_usd: Math.max(server.total_cost_micro_usd ?? 0, local?.total_cost_micro_usd ?? 0),
    last_context_window_tokens: CHAT_CONTEXT_WINDOW_TOKENS,
    message_count: Math.max(server.message_count ?? 0, local?.message_count ?? 0),
    model_call_count: Math.max(server.model_call_count ?? 0, local?.model_call_count ?? 0),
    tool_call_count: Math.max(server.tool_call_count ?? 0, local?.tool_call_count ?? 0),
    skill_call_count: Math.max(server.skill_call_count ?? 0, local?.skill_call_count ?? 0),
    mcp_call_count: Math.max(server.mcp_call_count ?? 0, local?.mcp_call_count ?? 0),
  };
}

// ── ActivityEvent-based functions ──────────────────────────────────────
// The backend projects context_usage / runner_completion / text_done
// (compress notice) as ActivityEvent with the corresponding
// `activity.stage` value. Token-usage fields are carried on `activity.meta`
// (with `prompt_tokens` / `completion_tokens` also available as direct
// Activity fields for the root task activity).

function numField(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

/**
 * Build an {@link ActivityUsage}-shaped object from an ActivityEvent so the
 * existing pure usage-derivation functions can be reused without duplication.
 *
 * Field mapping (activity):
 *   prompt_tokens         → activity.prompt_tokens (fallback: meta.prompt_tokens)
 *   completion_tokens     → activity.completion_tokens (fallback: meta.completion_tokens)
 *   total_tokens          → activity.meta.total_tokens
 *   max_tokens            → activity.meta.max_tokens
 *   context_prompt_tokens → activity.meta.context_prompt_tokens
 *   turn_total_tokens     → activity.meta.turn_total_tokens
 *
 * Returns undefined when no usage fields are present.
 */
function usageFromActivityEvent(ev: ActivityEvent): ActivityUsage | undefined {
  const act = ev.activity;
  const meta = act.meta ?? {};
  const prompt = numField(act.prompt_tokens) || numField(meta.prompt_tokens);
  const completion = numField(act.completion_tokens) || numField(meta.completion_tokens);
  const total = numField(meta.total_tokens);
  const maxTokens = numField(meta.max_tokens);
  // No usage payload at all — return undefined so callers can short-circuit.
  if (prompt === 0 && completion === 0 && total === 0 && maxTokens === 0) return undefined;
  const usage: ActivityUsage = {
    prompt_tokens: prompt,
    completion_tokens: completion,
    total_tokens: total,
  };
  if (maxTokens > 0) usage.max_tokens = maxTokens;
  if (typeof meta.context_prompt_tokens === 'number' && meta.context_prompt_tokens > 0) {
    usage.context_prompt_tokens = meta.context_prompt_tokens;
  }
  if (typeof meta.turn_total_tokens === 'number' && meta.turn_total_tokens > 0) {
    usage.turn_total_tokens = meta.turn_total_tokens;
  }
  return usage;
}

/** Detect a session-compress notice ActivityEvent (text_done + meta.kind). */
export function isSessionCompressNoticeFromActivityEvent(ev: ActivityEvent): boolean {
  if (ev.activity.stage !== 'text_done') return false;
  return ev.activity.meta?.kind === 'system.session.compress';
}

/**
 * Derive a {@link SessionContextPatch} directly from a v2 `system.notice`
 * context_usage payload Meta (useChatV2EventHandlers channel — the sole live
 * transport since the v1 activity bridge was removed). Meta keys match the
 * backend stream_consumer publishContextUsageStep map, with the prompt-assembly
 * breakdown under `context_budget` (backend ContextBudgetPayload).
 */
export function sessionContextPatchFromContextUsageMeta(
  meta: Record<string, unknown> | undefined,
): SessionContextPatch | null {
  if (!meta) return null;
  const prompt = numField(meta.prompt_tokens);
  const completion = numField(meta.completion_tokens);
  const total = numField(meta.total_tokens);
  const maxTokens = numField(meta.max_tokens);
  let usage: ActivityUsage | undefined;
  if (prompt !== 0 || completion !== 0 || total !== 0 || maxTokens !== 0) {
    usage = { prompt_tokens: prompt, completion_tokens: completion, total_tokens: total };
    if (maxTokens > 0) usage.max_tokens = maxTokens;
    if (typeof meta.context_prompt_tokens === 'number' && meta.context_prompt_tokens > 0) {
      usage.context_prompt_tokens = meta.context_prompt_tokens;
    }
    if (typeof meta.turn_total_tokens === 'number' && meta.turn_total_tokens > 0) {
      usage.turn_total_tokens = meta.turn_total_tokens;
    }
  }
  const patch = sessionContextPatchFromStepUsage(usage);
  const budget = parseContextBudgetMeta(meta.context_budget);
  if (budget) {
    return { ...(patch ?? {}), context_budget: budget };
  }
  return patch;
}

/**
 * Derive a {@link SessionContextPatch} from an ActivityEvent.
 *
 * Field mapping (envelope → activity):
 *   env.type === 'context_usage'      → ev.activity.stage === 'context_usage'
 *   env.type === 'runner_completion'  → ev.activity.stage === 'runner_completion'
 *   env.type === 'text_done' (compress) → ev.activity.stage === 'text_done' &&
 *     ev.activity.meta.kind === 'system.session.compress'
 *   env.usage.*                       → activity.{prompt_tokens,completion_tokens} +
 *     activity.meta.{total_tokens,max_tokens,context_prompt_tokens,turn_total_tokens}
 *   env.metadata (compress)           → ev.activity.meta
 */
export function sessionContextPatchFromActivityEvent(
  ev: ActivityEvent,
  prev?: Pick<Session, 'total_tokens' | 'max_context_used_ratio' | 'input_tokens' | 'output_tokens'>,
): SessionContextPatch | null {
  const stage = ev.activity.stage;
  if (stage === 'context_usage') {
    const usage = usageFromActivityEvent(ev);
    const patch = sessionContextPatchFromStepUsage(usage);
    // Prompt-assembly breakdown rides the same event (backend
    // ContextBudgetPayload); attach it so the SpiritStatusBar popup can render
    // the per-category composition without an extra fetch.
    const budget = parseContextBudgetMeta(ev.activity.meta?.context_budget);
    if (budget) {
      return { ...(patch ?? {}), context_budget: budget };
    }
    return patch;
  }
  if (stage === 'runner_completion') {
    const usage = usageFromActivityEvent(ev);
    return sessionContextPatchFromUsage(usage, prev);
  }
  if (isSessionCompressNoticeFromActivityEvent(ev)) {
    return sessionContextPatchFromCompressMeta(ev.activity.meta as Record<string, unknown> | undefined);
  }
  return null;
}
