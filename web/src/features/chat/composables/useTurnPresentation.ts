import { computed, toValue, type MaybeRefOrGetter } from 'vue';
import { useUiConfigStore } from '../../../stores/uiConfig';
import { useChatSessionStore } from '../../../stores/chat/sessionStore';
import { useActivityQueries } from './useActivityQueries';
import { isSystemInternalNotice } from '../noticeFilter';
import { isPlanAndExecuteTool } from '../executionCardHelpers';
import { cuSessionIdFromSteps } from '../../computeruse/useCuStepStream';
import type { Turn } from '../v2Types';

/** Read model for turn rendering; execution and session facts remain in their domain stores. */
export function useTurnPresentation(turn: MaybeRefOrGetter<Turn>) {
  const queries = useActivityQueries();
  const ui = useUiConfigStore();
  const sessions = useChatSessionStore();
  const forkable = computed(() => {
    const current = toValue(turn);
    const session = sessions.findSessionById(current.SessionID);
    const child = !!session?.parent_session_id?.trim() && !session.fork_from_turn_id?.trim();
    return (
      !child && !current.TeamStageID && current.SessionID === current.SpiritSessionID && current.Status !== 'running'
    );
  });
  const visibleSteps = computed(() =>
    queries.getTurnSteps(toValue(turn).ID).filter((step) => {
      if (isSystemInternalNotice(step.Kind, step.NoticeType)) return false;
      if (step.Kind === 'reply' && step.Status !== 'running' && !step.Content?.trim()) return false;
      return step.Kind !== 'action' || ui.showToolCalls || isPlanAndExecuteTool(step.ToolName);
    }),
  );
  const cuSessionId = computed(() => cuSessionIdFromSteps(visibleSteps.value));
  const isLiveTurn = computed(() => toValue(turn).Status === 'running');
  return { forkable, visibleSteps, cuSessionId, isLiveTurn };
}
