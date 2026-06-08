/**
 * OBS-02: Contextual loading message composable.
 *
 * Processes Spirit Envelope events and produces a single-line contextual
 * loading message that replaces (not appends) the previous one.
 * Messages are suppressed during WS replay to avoid flicker.
 */
import { ref, type Ref } from 'vue';
import type { Envelope } from '../../../realtime/envelope';
import { ORCHESTRATION_LOADING_MAP, AGENT_LOADING_MAP } from '../../../features/spirit/observabilityConstants';

export type ContextualMessage = {
  text: string;
  icon: string;
  color: string;
};

export function useContextualLoadingMessage(isReplaying: Ref<boolean>) {
  const loadingMessage = ref<ContextualMessage | null>(null);

  /**
   * Process a Spirit Envelope event.
   * Call this from the WS inbound sync handler.
   */
  function onSpiritEnvelope(envelope: Envelope): void {
    if (isReplaying.value) return;

    const envType = envelope.type;

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

    // 2. Check tool_call / tool_result events (agent-level)
    if (envType === 'tool_call' || envType === 'tool_result') {
      const tc = envelope.tool_call;
      if (!tc) return;

      const agentName = tc.agent_name || tc.agent_key || 'Agent';
      const config = AGENT_LOADING_MAP.find((c) => c.eventPattern === envType);
      if (!config) return;

      let text = config.messageTemplate;
      text = text.replace('{agentName}', agentName);

      if (envType === 'tool_call') {
        let displayLabel: string;
        if (tc.display_label) {
          displayLabel = tc.display_label;
        } else if (tc.activity_kind === 'skill') {
          displayLabel = tc.summary ? `运行 Skill ${tc.summary}` : '运行 Skill';
        } else {
          displayLabel = '执行操作';
        }
        text = text.replace('{displayLabel}', displayLabel);
      }

      if (envType === 'tool_result') {
        const rawDuration = tc.duration_ms ?? envelope.metadata?.duration_ms;
        const durationMs = typeof rawDuration === 'number' ? rawDuration : 0;
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
      envType === 'spirit_teams_all_completed'
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
    onSpiritEnvelope,
    clearMessage,
  };
}
