import { describe, expect, it } from 'vitest';
import {
  groupRoster,
  isPendingDecision,
  liveKindFromStep,
  mapNameToBizLine,
  stepToToolBeat,
  toolBeatText,
} from '../logic';

describe('isPendingDecision', () => {
  // E 阶段 W1b：对齐 decision_records 真实 outcome 词表
  // （asked/approved/rejected/timeout/resumed）——仅 asked 为待决。
  it('treats asked as pending (case-insensitive)', () => {
    expect(isPendingDecision('asked')).toBe(true);
    expect(isPendingDecision('Asked')).toBe(true);
    expect(isPendingDecision('ASKED')).toBe(true);
  });

  it('treats resolved/legacy outcomes as not pending', () => {
    expect(isPendingDecision('approved')).toBe(false);
    expect(isPendingDecision('rejected')).toBe(false);
    expect(isPendingDecision('timeout')).toBe(false);
    expect(isPendingDecision('resumed')).toBe(false);
    expect(isPendingDecision('executed')).toBe(false);
    // 旧过滤词与真实词表零交集（恒空 bug 根因），修复后不再判待决。
    expect(isPendingDecision('')).toBe(false);
    expect(isPendingDecision('pending')).toBe(false);
    expect(isPendingDecision('awaiting')).toBe(false);
  });
});

describe('mapNameToBizLine', () => {
  it('maps keyword hits to lanes', () => {
    expect(mapNameToBizLine('NovaTrader 盯盘组')).toBe('novatrader');
    expect(mapNameToBizLine('hotcraft-热点')).toBe('hotcraft');
    expect(mapNameToBizLine('综合助手部')).toBe('assistant');
    expect(mapNameToBizLine('ecmove 刊登组')).toBe('ecmove');
  });

  it('is case-insensitive on latin keywords', () => {
    expect(mapNameToBizLine('CONTENT team')).toBe('hotcraft');
    expect(mapNameToBizLine('Assistant Pool')).toBe('assistant');
  });

  it('returns empty for unrecognized names and empty input', () => {
    expect(mapNameToBizLine('保洁部')).toBe('');
    expect(mapNameToBizLine('')).toBe('');
  });
});

describe('groupRoster', () => {
  const depts = [
    { id: 'd-trade', name: 'Novatrader 交易部' },
    { id: 'd-ops', name: '综合部' },
  ];
  const positions = [
    { id: 'p-quant', name: '量化岗', parentId: 'd-trade' },
    { id: 'p-sec', name: '秘书岗', parentId: 'd-ops' },
  ];

  it('attaches agents to positions with dept + bizLine derived', () => {
    const m = groupRoster(depts, positions, [
      { agent_key: 'q1', display_name: 'Quant One', icon: '', org_node_id: 'p-quant' },
    ]);
    const agent = m.depts[0].positions[0].agents[0];
    expect(agent.key).toBe('q1');
    expect(agent.deptId).toBe('d-trade');
    expect(agent.deptName).toBe('Novatrader 交易部');
    expect(agent.bizLine).toBe('novatrader');
    expect(m.unassigned).toHaveLength(0);
  });

  it('floats agents bound directly to a dept into floatAgents', () => {
    const m = groupRoster(depts, positions, [
      { agent_key: 'lead', display_name: 'Dept Lead', icon: '', org_node_id: 'd-ops' },
    ]);
    expect(m.depts[1].floatAgents).toHaveLength(1);
    expect(m.depts[1].floatAgents[0].positionId).toBe('');
  });

  it('routes unknown / dangling org nodes to unassigned with name-derived bizLine', () => {
    const m = groupRoster(depts, positions, [
      { agent_key: 'a-hot', display_name: '热点小编', icon: '', org_node_id: 'p-gone' },
      { agent_key: 'a-none', display_name: '', icon: '' },
    ]);
    expect(m.unassigned).toHaveLength(2);
    expect(m.unassigned[0].bizLine).toBe('hotcraft');
    // display_name 空 → 退回 agent_key
    expect(m.unassigned[1].name).toBe('a-none');
  });

  it('drops soft-deleted agents entirely', () => {
    const m = groupRoster(depts, positions, [
      { agent_key: 'dead', display_name: 'Gone', icon: '', org_node_id: 'p-quant', deleted_at: '2026-01-01' },
    ]);
    expect(m.depts[0].positions[0].agents).toHaveLength(0);
    expect(m.unassigned).toHaveLength(0);
  });

  it('keeps depts with zero positions / zero agents (empty sections render)', () => {
    const m = groupRoster(depts, positions, []);
    expect(m.depts).toHaveLength(2);
    expect(m.depts[0].positions[0].agents).toEqual([]);
  });
});

describe('liveKindFromStep', () => {
  it('waiting overrides everything', () => {
    expect(liveKindFromStep('thinking', 'running', '', true)).toBe('waiting');
    expect(liveKindFromStep('action', 'failed', 'E_X', true)).toBe('waiting');
  });

  it('failed status or error code wins over kind', () => {
    expect(liveKindFromStep('action', 'failed', '', false)).toBe('error');
    expect(liveKindFromStep('action', 'completed', 'E_TOOL', false)).toBe('error');
  });

  it('maps kinds to live words', () => {
    expect(liveKindFromStep('thinking', 'running', '', false)).toBe('thinking');
    expect(liveKindFromStep('action', 'running', '', false)).toBe('working');
    expect(liveKindFromStep('reply', 'running', '', false)).toBe('working');
    expect(liveKindFromStep('system', 'running', '', false)).toBe('idle');
  });
});

describe('stepToToolBeat / toolBeatText', () => {
  it('returns null when no tool name', () => {
    expect(
      stepToToolBeat({
        ToolName: '',
        ToolDurationMs: 10,
        ToolErrorCode: '',
        CompletedAt: null,
        StartedAt: 's',
        Status: 'completed',
      }),
    ).toBeNull();
  });

  it('prefers CompletedAt, falls back to StartedAt', () => {
    const beat = stepToToolBeat({
      ToolName: 'search',
      ToolDurationMs: 1500,
      ToolErrorCode: '',
      CompletedAt: '2026-10-24T08:00:01Z',
      StartedAt: '2026-10-24T08:00:00Z',
      Status: 'completed',
    });
    expect(beat?.at).toBe('2026-10-24T08:00:01Z');
    expect(beat?.toolName).toBe('search');
  });

  it('formats duration without trailing .0', () => {
    expect(toolBeatText({ toolName: 'search', durationMs: 1500, errorCode: '', at: '' })).toBe('search · 1.5s');
    expect(toolBeatText({ toolName: 'read', durationMs: 2000, errorCode: '', at: '' })).toBe('read · 2s');
  });
});
