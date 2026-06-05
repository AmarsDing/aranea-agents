import type { Envelope, EnvelopeUsage } from '../../realtime/envelope';
import type { Session } from '../session/types';
import { contextRatioFromPrompt, contextStatusFromRatio } from '../session/contextMetrics';

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
  >
> & {
  // Reserved for future server-driven prompt breakdown
};

/** Prompt tokens used for context-window fill (max in turn), distinct from billing aggregates. */
export function contextPromptTokensFromUsage(usage: EnvelopeUsage | undefined): number {
  if (!usage) return 0;
  const explicit = usage.context_prompt_tokens ?? 0;
  if (explicit > 0) return explicit;
  return usage.prompt_tokens ?? 0;
}

export function contextRatioFromUsage(usage: EnvelopeUsage | undefined): number | null {
  const prompt = contextPromptTokensFromUsage(usage);
  const window = usage?.max_tokens ?? 0;
  return contextRatioFromPrompt(prompt, window);
}

export { contextStatusFromRatio } from '../session/contextMetrics';

export function isSessionCompressNotice(env: Envelope): boolean {
  if (env.type !== 'text_done') return false;
  const md = env.metadata as Record<string, unknown> | undefined;
  return md?.kind === 'system.session.compress';
}

/** Mid-turn ReAct sub-step: update context bar only (no session total_tokens increment). */
export function sessionContextPatchFromStepUsage(usage: EnvelopeUsage | undefined): SessionContextPatch | null {
  const ratio = contextRatioFromUsage(usage);
  if (ratio == null || !usage) return null;

  const contextPrompt = contextPromptTokensFromUsage(usage);
  const patch: SessionContextPatch = {
    context_used_ratio: ratio,
    context_used_tokens: contextPrompt,
    context_status: contextStatusFromRatio(ratio),
  };

  if (usage.max_tokens != null && usage.max_tokens > 0) {
    patch.last_context_window_tokens = usage.max_tokens;
  }

  return patch;
}

export function sessionContextPatchFromUsage(
  usage: EnvelopeUsage | undefined,
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

  if (usage.max_tokens != null && usage.max_tokens > 0) {
    patch.last_context_window_tokens = usage.max_tokens;
  }

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
  if (typeof meta.context_window === 'number') {
    patch.last_context_window_tokens = meta.context_window;
  }

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
    last_context_window_tokens: server.last_context_window_tokens,
    message_count: Math.max(server.message_count ?? 0, local?.message_count ?? 0),
    model_call_count: Math.max(server.model_call_count ?? 0, local?.model_call_count ?? 0),
    tool_call_count: Math.max(server.tool_call_count ?? 0, local?.tool_call_count ?? 0),
    skill_call_count: Math.max(server.skill_call_count ?? 0, local?.skill_call_count ?? 0),
    mcp_call_count: Math.max(server.mcp_call_count ?? 0, local?.mcp_call_count ?? 0),
  };
}

export function sessionContextPatchFromEnvelope(
  env: Envelope,
  prev?: Pick<Session, 'total_tokens' | 'max_context_used_ratio' | 'input_tokens' | 'output_tokens'>,
): SessionContextPatch | null {
  if (env.type === 'context_usage' && env.usage) {
    return sessionContextPatchFromStepUsage(env.usage);
  }
  if (env.type === 'runner_completion' && env.usage) {
    return sessionContextPatchFromUsage(env.usage, prev);
  }
  if (isSessionCompressNotice(env)) {
    return sessionContextPatchFromCompressMeta(env.metadata as Record<string, unknown> | undefined);
  }
  return null;
}
