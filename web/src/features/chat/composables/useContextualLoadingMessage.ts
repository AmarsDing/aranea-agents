/**
 * OBS-02: Contextual loading message composable.
 *
 * Processes Spirit ActivityEvents and produces a single-line contextual
 * loading message that replaces (not appends) the previous one.
 * Messages are suppressed during reconnect hydrate to avoid flicker.
 */
import { ref, type Ref } from 'vue';
import type { ActivityEvent } from '../../../realtime/activityEvent';
import { ORCHESTRATION_LOADING_MAP, AGENT_LOADING_MAP } from '../../../features/spirit/observabilityConstants';

export type ContextualMessage = {
  text: string;
  icon: string;
  color: string;
};

/**
 * Map an ActivityEvent's kind + stage to the legacy envelope type string
 * that the contextual loading message logic expects. Returns '' when no
 * mapping exists.
 */
function activityEventToLoadingType(ev: ActivityEvent): string {
  const { kind, stage } = ev.activity;
  // Orchestration phases
  if (kind === 'plan') return 'spirit_plan_created';
  if (kind === 'session') {
    if (stage === 'plan_created') return 'spirit_plan_created';
    if (stage === 'allocation_created') return 'spirit_allocation_created';
    if (stage === 'orchestration_started') return 'spirit_orchestration_started';
    if (stage === 'synthesis_completed') return 'spirit_synthesis_completed';
    if (stage === 'orchestration_completed') return 'butler.orchestration.completed';
    if (stage === 'orchestration_failed') return 'butler.orchestration.failed';
  }
  if (kind === 'notice' && stage === 'allocation_created') return 'spirit_allocation_created';
  // Team stages
  if (kind === 'team_stage') {
    if (stage === 'assembled') return 'spirit_team_assembled';
    if (stage === 'progress') return 'spirit_team_progress';
    if (stage === 'interrupted') return 'spirit_team_interrupted';
    if (stage === 'cancelled') return 'spirit_team_cancelled';
    if (stage === 'completed' || stage === 'finished') return 'spirit_team_completed';
    if (stage === 'failed') return 'spirit_team_failed';
    if (stage === 'orchestration_started') return 'spirit_orchestration_started';
    if (stage === 'all_completed' || stage === 'summary') return 'spirit_teams_all_completed';
  }
  // Tool events (kind=action)
  if (kind === 'action') {
    if (ev.event === 'created') return 'tool_call';
    if (ev.event === 'completed' || ev.event === 'failed') return 'tool_result';
  }
  return '';
}

export function useContextualLoadingMessage(isReplaying: Ref<boolean>) {
  const loadingMessage = ref<ContextualMessage | null>(null);

  /**
   * Activity-First: Process an ActivityEvent and produce a contextual
   * loading message. Maps kind+stage to the legacy event type string and
   * uses the same message template lookup as the legacy envelope path.
   *
   * Call this from the WS inbound sync handler (useChatWorkspace routes
   * Spirit ActivityEvents here via onSpiritActivityEvent).
   */
  function onSpiritActivityEvent(ev: ActivityEvent): void {
    if (isReplaying.value) return;

    const envType = activityEventToLoadingType(ev);
    if (!envType) return;

    // 1. Check orchestration-phase events
    const orchestrationConfig = ORCHESTRATION_LOADING_MAP.find((c) => c.eventPattern === envType);
    if (orchestrationConfig) {
      loadingMessage.value = {
        text: orchestrationConfig.messageTemplate,
        icon: orchestrationConfig.icon,
        color: orchestrationConfig.color,
      };
      return;
    }

    // 2. Check tool_call / tool_result events (kind=action)
    if (envType === 'tool_call' || envType === 'tool_result') {
      const act = ev.activity;
      const agentName = act.agent_name || act.agent_key || 'Agent';
      const config = AGENT_LOADING_MAP.find((c) => c.eventPattern === envType);
      if (!config) return;

      let text = config.messageTemplate;
      text = text.replace('{agentName}', agentName);

      if (envType === 'tool_call') {
        const meta = act.meta ?? {};
        let displayLabel: string;
        const metaLabel = typeof meta.display_label === 'string' ? meta.display_label : '';
        if (metaLabel) {
          displayLabel = metaLabel;
        } else if (act.label) {
          displayLabel = act.label;
        } else if (String(meta.activity_kind ?? '') === 'skill') {
          const summary = typeof meta.summary === 'string' ? meta.summary : '';
          displayLabel = summary ? `运行 Skill ${summary}` : '运行 Skill';
        } else {
          displayLabel = act.tool_name || '执行操作';
        }
        text = text.replace('{displayLabel}', displayLabel);
      }

      if (envType === 'tool_result') {
        const durationMs = typeof act.tool_duration_ms === 'number' ? act.tool_duration_ms : 0;
        const durationSec = Math.round(durationMs / 1000);
        text = text.replace('{durationSec}', String(durationSec));
      }

      loadingMessage.value = {
        text,
        icon: config.icon,
        color: config.color,
      };
      return;
    }

    // 3. Clear message on team completion/failure
    if (
      envType === 'spirit_team_completed' ||
      envType === 'spirit_team_failed' ||
      envType === 'spirit_teams_all_completed' ||
      envType === 'butler.orchestration.completed' ||
      envType === 'butler.orchestration.failed'
    ) {
      loadingMessage.value = null;
    }
  }

  /** Manually clear the loading message. */
  function clearMessage(): void {
    loadingMessage.value = null;
  }

  return {
    loadingMessage,
    onSpiritActivityEvent,
    clearMessage,
  };
}
