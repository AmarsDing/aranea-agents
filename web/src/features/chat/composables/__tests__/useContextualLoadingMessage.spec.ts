import { describe, it, expect } from 'vitest';
import { ref } from 'vue';
import { useContextualLoadingMessage } from '../useContextualLoadingMessage';
import type { Envelope } from '../../../../realtime/envelope';

function makeEnvelope(overrides: Partial<Envelope> & Pick<Envelope, 'type'>): Envelope {
  return {
    id: 'env-1',
    author: 'system',
    session_id: 's1',
    timestamp: '2026-06-08T00:00:00Z',
    version: 1,
    ...overrides,
  };
}

describe('useContextualLoadingMessage', () => {
  it('initially loadingMessage is null', () => {
    const isReplaying = ref(false);
    const { loadingMessage } = useContextualLoadingMessage(isReplaying);

    expect(loadingMessage.value).toBeNull();
  });

  describe('orchestration events', () => {
    it('butler.orchestration.started produces correct message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(makeEnvelope({ type: 'butler.orchestration.started' }));

      expect(loadingMessage.value).not.toBeNull();
      expect(loadingMessage.value!.text).toBe('正在处理任务…');
      expect(loadingMessage.value!.icon).toBe('sync');
      expect(loadingMessage.value!.color).toBe('grey');
    });

    it('spirit_plan_created produces correct message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(makeEnvelope({ type: 'spirit_plan_created' }));

      expect(loadingMessage.value).not.toBeNull();
      expect(loadingMessage.value!.text).toBe('正在分析任务复杂度…');
      expect(loadingMessage.value!.icon).toBe('search');
      expect(loadingMessage.value!.color).toBe('blue');
    });

    it('spirit_allocation_created produces correct message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(makeEnvelope({ type: 'spirit_allocation_created' }));

      expect(loadingMessage.value!.text).toBe('正在分配 Agent 角色…');
      expect(loadingMessage.value!.icon).toBe('people');
      expect(loadingMessage.value!.color).toBe('purple');
    });

    it('spirit_orchestration_started produces correct message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(makeEnvelope({ type: 'spirit_orchestration_started' }));

      expect(loadingMessage.value!.text).toBe('正在编排执行流程…');
      expect(loadingMessage.value!.icon).toBe('construction');
      expect(loadingMessage.value!.color).toBe('orange');
    });
  });

  describe('tool_call events', () => {
    it('tool_call produces agent-level message with correct agentName and displayLabel', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(
        makeEnvelope({
          type: 'tool_call',
          tool_call: {
            id: 'tc-1',
            name: 'web_search',
            arguments_json: '{}',
            status: 'running',
            agent_name: 'Researcher',
            display_label: '搜索网页',
          },
        }),
      );

      expect(loadingMessage.value).not.toBeNull();
      expect(loadingMessage.value!.text).toBe('Researcher 正在搜索网页…');
      expect(loadingMessage.value!.icon).toBe('bolt');
      expect(loadingMessage.value!.color).toBe('blue');
    });

    it('tool_call with activity_kind=skill and summary produces "运行 Skill {summary}" message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(
        makeEnvelope({
          type: 'tool_call',
          tool_call: {
            id: 'tc-2',
            name: 'run_skill',
            arguments_json: '{}',
            status: 'running',
            agent_name: 'Coder',
            activity_kind: 'skill',
            summary: '代码审查',
          },
        }),
      );

      expect(loadingMessage.value!.text).toBe('Coder 正在运行 Skill 代码审查…');
    });

    it('tool_call with activity_kind=skill without summary produces "运行 Skill" message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(
        makeEnvelope({
          type: 'tool_call',
          tool_call: {
            id: 'tc-3',
            name: 'run_skill',
            arguments_json: '{}',
            status: 'running',
            agent_name: 'Coder',
            activity_kind: 'skill',
          },
        }),
      );

      expect(loadingMessage.value!.text).toBe('Coder 正在运行 Skill…');
    });

    it('tool_call without display_label or skill uses default "执行操作"', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(
        makeEnvelope({
          type: 'tool_call',
          tool_call: {
            id: 'tc-4',
            name: 'some_tool',
            arguments_json: '{}',
            status: 'running',
            agent_name: 'Agent-X',
          },
        }),
      );

      expect(loadingMessage.value!.text).toBe('Agent-X 正在执行操作…');
    });

    it('tool_call falls back to agent_key when agent_name is absent', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(
        makeEnvelope({
          type: 'tool_call',
          tool_call: {
            id: 'tc-5',
            name: 'some_tool',
            arguments_json: '{}',
            status: 'running',
            agent_key: 'researcher',
            display_label: '搜索',
          },
        }),
      );

      expect(loadingMessage.value!.text).toBe('researcher 正在搜索…');
    });

    it('tool_call falls back to "Agent" when neither agent_name nor agent_key is set', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(
        makeEnvelope({
          type: 'tool_call',
          tool_call: {
            id: 'tc-6',
            name: 'some_tool',
            arguments_json: '{}',
            status: 'running',
            display_label: '搜索',
          },
        }),
      );

      expect(loadingMessage.value!.text).toBe('Agent 正在搜索…');
    });
  });

  describe('tool_result events', () => {
    it('tool_result produces completion message with duration', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(
        makeEnvelope({
          type: 'tool_result',
          tool_call: {
            id: 'tc-1',
            name: 'web_search',
            arguments_json: '{}',
            status: 'completed',
            agent_name: 'Researcher',
            duration_ms: 3500,
          },
        }),
      );

      expect(loadingMessage.value).not.toBeNull();
      expect(loadingMessage.value!.text).toBe('Researcher 完成，耗时 4s');
      expect(loadingMessage.value!.icon).toBe('check_circle');
      expect(loadingMessage.value!.color).toBe('green');
    });

    it('tool_result with zero duration shows 0s', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(
        makeEnvelope({
          type: 'tool_result',
          tool_call: {
            id: 'tc-2',
            name: 'some_tool',
            arguments_json: '{}',
            status: 'completed',
            agent_name: 'Agent',
          },
        }),
      );

      expect(loadingMessage.value!.text).toBe('Agent 完成，耗时 0s');
    });
  });

  describe('team completion events', () => {
    it('spirit_team_completed clears loadingMessage', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(makeEnvelope({ type: 'butler.orchestration.started' }));
      expect(loadingMessage.value).not.toBeNull();

      onSpiritEnvelope(makeEnvelope({ type: 'spirit_team_completed' }));
      expect(loadingMessage.value).toBeNull();
    });

    it('spirit_team_failed clears loadingMessage', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(makeEnvelope({ type: 'butler.orchestration.started' }));

      onSpiritEnvelope(makeEnvelope({ type: 'spirit_team_failed' }));
      expect(loadingMessage.value).toBeNull();
    });

    it('spirit_teams_all_completed clears loadingMessage', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(makeEnvelope({ type: 'butler.orchestration.started' }));

      onSpiritEnvelope(makeEnvelope({ type: 'spirit_teams_all_completed' }));
      expect(loadingMessage.value).toBeNull();
    });
  });

  describe('replay suppression', () => {
    it('isReplaying=true suppresses all messages', () => {
      const isReplaying = ref(true);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(makeEnvelope({ type: 'butler.orchestration.started' }));
      expect(loadingMessage.value).toBeNull();

      onSpiritEnvelope(
        makeEnvelope({
          type: 'tool_call',
          tool_call: {
            id: 'tc-1',
            name: 'some_tool',
            arguments_json: '{}',
            status: 'running',
            agent_name: 'Agent',
            display_label: '搜索',
          },
        }),
      );
      expect(loadingMessage.value).toBeNull();
    });
  });

  describe('clearMessage', () => {
    it('clearMessage manually clears the loading message', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope, clearMessage } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(makeEnvelope({ type: 'butler.orchestration.started' }));
      expect(loadingMessage.value).not.toBeNull();

      clearMessage();
      expect(loadingMessage.value).toBeNull();
    });
  });

  describe('edge cases', () => {
    it('tool_call without tool_call field is ignored', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(makeEnvelope({ type: 'tool_call' }));
      expect(loadingMessage.value).toBeNull();
    });

    it('tool_result without tool_call field is ignored', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(makeEnvelope({ type: 'tool_result' }));
      expect(loadingMessage.value).toBeNull();
    });

    it('unknown event type does not change loadingMessage', () => {
      const isReplaying = ref(false);
      const { loadingMessage, onSpiritEnvelope } = useContextualLoadingMessage(isReplaying);

      onSpiritEnvelope(makeEnvelope({ type: 'text_delta' }));
      expect(loadingMessage.value).toBeNull();
    });
  });
});
