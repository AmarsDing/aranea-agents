import { describe, it, expect } from 'vitest';
import { ref } from 'vue';
import { useContextualLoadingMessage } from '../useContextualLoadingMessage';
import type { ActivityEvent, Activity, ActivityKind, ActivityStatus, ActivityEventType } from '../../../../realtime/activityEvent';

/**
 * Build a minimal ActivityEvent for testing. Only the fields accessed by
 * useContextualLoadingMessage need to be set; the rest are defaulted.
 */
function makeActivityEvent(
  overrides: {
    event?: ActivityEventType;
    kind?: ActivityKind;
    stage?: string;
    agent_name?: string;
    agent_key?: string;
    label?: string;
    tool_name?: string;
    tool_duration_ms?: number;
    meta?: Record<string, unknown>;
  },
): ActivityEvent {
  const activity: Activity = {
    id: 'a-1',
    kind: overrides.kind ?? 'task',
    status: 'running' as ActivityStatus,
    session_id: 's1',
    turn_id: 't1',
    parent_activity_id: '',
    timestamp: '2026-06-08T00:00:00Z',
    duration_ms: 0,
    seq: 1,
    prompt_tokens: 0,
    completion_tokens: 0,
    content: '',
    reasoning: '',
    tool_name: overrides.tool_name ?? '',
    tool_category: 'other',
    tool_call_id: '',
    tool_arguments: '',
    tool_result: '',
    tool_duration_ms: overrides.tool_duration_ms ?? 0,
    tool_error_code: '',
    stage: overrides.stage ?? '',
    child_board_id: '',
    spirit_session_id: '',
    team_id: '',
    dag_node_id: '',
    depends_on: [],
    agent_key: overrides.agent_key ?? '',
    agent_name: overrides.agent_name ?? '',
    collapsed: false,
    label: overrides.label ?? '',
    meta: overrides.meta ?? {},
  };
  return {
    event: overrides.event ?? 'created',
    activity,
  };
}

describe('useContextualLoadingMessage', () => {
  it('initially loadingMessage is null', () => {
    const isReplaying = ref(false);
    const { loadingMessage } = useContextualLoadingMessage(isReplaying);

    expect(loadingMessage.value).toBeNull();
  });

  describe('orchestration events', () => {
    it('spirit_plan_created (kind=session, stage=plan_created) produces correct message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(makeActivityEvent({ kind: 'session', stage: 'plan_created' }));

      expect(loadingMessage.value).not.toBeNull();
      expect(loadingMessage.value!.text).toBe('正在分析任务复杂度…');
      expect(loadingMessage.value!.icon).toBe('search');
      expect(loadingMessage.value!.color).toBe('blue');
    });

    it('spirit_plan_created (kind=plan) produces correct message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(makeActivityEvent({ kind: 'plan' }));

      expect(loadingMessage.value!.text).toBe('正在分析任务复杂度…');
    });

    it('spirit_allocation_created (kind=session, stage=allocation_created) produces correct message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(makeActivityEvent({ kind: 'session', stage: 'allocation_created' }));

      expect(loadingMessage.value!.text).toBe('正在分配 Agent 角色…');
      expect(loadingMessage.value!.icon).toBe('people');
      expect(loadingMessage.value!.color).toBe('purple');
    });

    it('spirit_allocation_created (kind=notice, stage=allocation_created) produces correct message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(makeActivityEvent({ kind: 'notice', stage: 'allocation_created' }));

      expect(loadingMessage.value!.text).toBe('正在分配 Agent 角色…');
    });

    it('spirit_orchestration_started (kind=session) produces correct message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(makeActivityEvent({ kind: 'session', stage: 'orchestration_started' }));

      expect(loadingMessage.value!.text).toBe('正在编排执行流程…');
      expect(loadingMessage.value!.icon).toBe('construction');
      expect(loadingMessage.value!.color).toBe('orange');
    });

    it('spirit_orchestration_started (kind=team_stage) produces correct message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(makeActivityEvent({ kind: 'team_stage', stage: 'orchestration_started' }));

      expect(loadingMessage.value!.text).toBe('正在编排执行流程…');
    });
  });

  describe('tool_call events (kind=action, event=created)', () => {
    it('produces agent-level message with correct agentName and displayLabel from meta', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(
        makeActivityEvent({
          kind: 'action',
          event: 'created',
          agent_name: 'Researcher',
          tool_name: 'web_search',
          meta: { display_label: '搜索网页' },
        }),
      );

      expect(loadingMessage.value).not.toBeNull();
      expect(loadingMessage.value!.text).toBe('Researcher 正在搜索网页…');
      expect(loadingMessage.value!.icon).toBe('bolt');
      expect(loadingMessage.value!.color).toBe('blue');
    });

    it('with activity_kind=skill and summary produces "运行 Skill {summary}" message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(
        makeActivityEvent({
          kind: 'action',
          event: 'created',
          agent_name: 'Coder',
          tool_name: 'run_skill',
          meta: { activity_kind: 'skill', summary: '代码审查' },
        }),
      );

      expect(loadingMessage.value!.text).toBe('Coder 正在运行 Skill 代码审查…');
    });

    it('with activity_kind=skill without summary produces "运行 Skill" message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(
        makeActivityEvent({
          kind: 'action',
          event: 'created',
          agent_name: 'Coder',
          tool_name: 'run_skill',
          meta: { activity_kind: 'skill' },
        }),
      );

      expect(loadingMessage.value!.text).toBe('Coder 正在运行 Skill…');
    });

    it('uses label when meta.display_label is absent', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(
        makeActivityEvent({
          kind: 'action',
          event: 'created',
          agent_name: 'Agent-X',
          tool_name: 'some_tool',
          label: '自定义操作',
        }),
      );

      expect(loadingMessage.value!.text).toBe('Agent-X 正在自定义操作…');
    });

    it('without display_label/skill/label uses tool_name as default label', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(
        makeActivityEvent({
          kind: 'action',
          event: 'created',
          agent_name: 'Agent-X',
          tool_name: 'some_tool',
        }),
      );

      expect(loadingMessage.value!.text).toBe('Agent-X 正在some_tool…');
    });

    it('without any label source uses default "执行操作"', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(
        makeActivityEvent({
          kind: 'action',
          event: 'created',
          agent_name: 'Agent-X',
        }),
      );

      expect(loadingMessage.value!.text).toBe('Agent-X 正在执行操作…');
    });

    it('falls back to agent_key when agent_name is absent', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(
        makeActivityEvent({
          kind: 'action',
          event: 'created',
          agent_key: 'researcher',
          meta: { display_label: '搜索' },
        }),
      );

      expect(loadingMessage.value!.text).toBe('researcher 正在搜索…');
    });

    it('falls back to "Agent" when neither agent_name nor agent_key is set', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(
        makeActivityEvent({
          kind: 'action',
          event: 'created',
          meta: { display_label: '搜索' },
        }),
      );

      expect(loadingMessage.value!.text).toBe('Agent 正在搜索…');
    });
  });

  describe('tool_result events (kind=action, event=completed)', () => {
    it('produces completion message with duration', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(
        makeActivityEvent({
          kind: 'action',
          event: 'completed',
          agent_name: 'Researcher',
          tool_name: 'web_search',
          tool_duration_ms: 3500,
        }),
      );

      expect(loadingMessage.value).not.toBeNull();
      expect(loadingMessage.value!.text).toBe('Researcher 完成，耗时 4s');
      expect(loadingMessage.value!.icon).toBe('check_circle');
      expect(loadingMessage.value!.color).toBe('green');
    });

    it('with zero duration shows 0s', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(
        makeActivityEvent({
          kind: 'action',
          event: 'completed',
          agent_name: 'Agent',
          tool_name: 'some_tool',
          tool_duration_ms: 0,
        }),
      );

      expect(loadingMessage.value!.text).toBe('Agent 完成，耗时 0s');
    });

    it('with undefined tool_duration_ms shows 0s', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(
        makeActivityEvent({
          kind: 'action',
          event: 'completed',
          agent_name: 'Agent',
          tool_name: 'some_tool',
          // tool_duration_ms intentionally omitted
        }),
      );

      expect(loadingMessage.value!.text).toBe('Agent 完成，耗时 0s');
    });

    it('event=failed also produces tool_result message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(
        makeActivityEvent({
          kind: 'action',
          event: 'failed',
          agent_name: 'Agent',
          tool_name: 'some_tool',
          tool_duration_ms: 1500,
        }),
      );

      expect(loadingMessage.value!.text).toBe('Agent 完成，耗时 2s');
    });
  });

  describe('team completion events', () => {
    it('spirit_team_completed (stage=completed) clears loadingMessage', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(makeActivityEvent({ kind: 'session', stage: 'orchestration_started' }));
      expect(loadingMessage.value).not.toBeNull();

      onSpiritActivityEvent(makeActivityEvent({ kind: 'team_stage', stage: 'completed', event: 'completed' }));
      expect(loadingMessage.value).toBeNull();
    });

    it('spirit_team_completed (stage=finished) clears loadingMessage', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(makeActivityEvent({ kind: 'session', stage: 'orchestration_started' }));

      onSpiritActivityEvent(makeActivityEvent({ kind: 'team_stage', stage: 'finished', event: 'completed' }));
      expect(loadingMessage.value).toBeNull();
    });

    it('spirit_team_failed clears loadingMessage', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(makeActivityEvent({ kind: 'session', stage: 'orchestration_started' }));

      onSpiritActivityEvent(makeActivityEvent({ kind: 'team_stage', stage: 'failed', event: 'failed' }));
      expect(loadingMessage.value).toBeNull();
    });

    it('spirit_teams_all_completed (stage=all_completed) clears loadingMessage', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(makeActivityEvent({ kind: 'session', stage: 'orchestration_started' }));

      onSpiritActivityEvent(makeActivityEvent({ kind: 'team_stage', stage: 'all_completed', event: 'completed' }));
      expect(loadingMessage.value).toBeNull();
    });

    it('spirit_teams_all_completed (stage=summary) clears loadingMessage', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(makeActivityEvent({ kind: 'session', stage: 'orchestration_started' }));

      onSpiritActivityEvent(makeActivityEvent({ kind: 'team_stage', stage: 'summary', event: 'completed' }));
      expect(loadingMessage.value).toBeNull();
    });
  });

  describe('replay suppression', () => {
    it('isReplaying=true suppresses all messages', () => {
      const isReplaying = ref(true);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(makeActivityEvent({ kind: 'session', stage: 'orchestration_started' }));
      expect(loadingMessage.value).toBeNull();

      onSpiritActivityEvent(
        makeActivityEvent({
          kind: 'action',
          event: 'created',
          agent_name: 'Agent',
          meta: { display_label: '搜索' },
        }),
      );
      expect(loadingMessage.value).toBeNull();
    });
  });

  describe('clearMessage', () => {
    it('clearMessage manually clears the loading message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent, clearMessage } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(makeActivityEvent({ kind: 'session', stage: 'orchestration_started' }));
      expect(loadingMessage.value).not.toBeNull();

      clearMessage();
      expect(loadingMessage.value).toBeNull();
    });
  });

  describe('edge cases', () => {
    it('unmapped kind (task) does not change loadingMessage', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(makeActivityEvent({ kind: 'task', event: 'created' }));
      expect(loadingMessage.value).toBeNull();
    });

    it('unmapped stage does not change loadingMessage', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(makeActivityEvent({ kind: 'session', stage: 'unknown_stage' }));
      expect(loadingMessage.value).toBeNull();
    });

    it('kind=action with event=streaming does not produce tool_call/tool_result message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritActivityEvent } = useContextualLoadingMessage(isReplaying);

      onSpiritActivityEvent(
        makeActivityEvent({
          kind: 'action',
          event: 'streaming',
          agent_name: 'Agent',
        }),
      );
      expect(loadingMessage.value).toBeNull();
    });
  });
});
