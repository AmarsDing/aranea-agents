import { describe, expect, it } from 'vitest';
import type { DecisionRecordItem } from '../../decision/types';
import type { Opportunity, ReviewTask } from '../../mvp/api';
import {
  buildListingLane,
  buildPublishLane,
  buildTradeLane,
  groupRoster,
  isPendingDecision,
  liveKindFromStep,
  mapNameToBizLine,
  stepToToolBeat,
  toolBeatText,
} from '../logic';

describe('isPendingDecision', () => {
  it('treats empty / pending / awaiting as pending (case-insensitive)', () => {
    expect(isPendingDecision('')).toBe(true);
    expect(isPendingDecision('pending')).toBe(true);
    expect(isPendingDecision('PENDING')).toBe(true);
    expect(isPendingDecision('Awaiting')).toBe(true);
  });

  it('treats resolved outcomes as done', () => {
    expect(isPendingDecision('approved')).toBe(false);
    expect(isPendingDecision('rejected')).toBe(false);
    expect(isPendingDecision('executed')).toBe(false);
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

describe('buildTradeLane', () => {
  const mk = (over: Partial<DecisionRecordItem>): DecisionRecordItem => ({
    decision_key: 'k1',
    category: 'hitl_approval',
    scenario: 'buy',
    reasoning: 'r',
    outcome: 'pending',
    actor_type: 'agent',
    actor_key: 'q1',
    created_at: '2026-10-24T01:00:00Z',
    ...over,
  });

  it('keeps only pending records; decisions are goto-only (no write path)', () => {
    const lane = buildTradeLane([mk({}), mk({ decision_key: 'k2', outcome: 'approved' })], 'now', false);
    expect(lane.count).toBe(1);
    expect(lane.items[0].action).toBe('goto');
    expect(lane.items[0].to).toBe('/team');
    expect(lane.items[0].danger).toBe(true);
  });

  it('caps rows at 5 while count reflects the full backlog', () => {
    const many = Array.from({ length: 7 }, (_, i) => mk({ decision_key: `k${i}` }));
    const lane = buildTradeLane(many, 'now', false);
    expect(lane.items).toHaveLength(5);
    expect(lane.count).toBe(7);
  });

  it('falls back to ? for missing actor and slices long summaries', () => {
    const lane = buildTradeLane([mk({ actor_key: '', reasoning: 'x'.repeat(120) })], 'now', true);
    expect(lane.items[0].who).toBe('?');
    expect(lane.items[0].summary).toHaveLength(80);
    expect(lane.degraded).toBe(true);
  });
});

describe('buildPublishLane', () => {
  const mk = (over: Partial<ReviewTask>): ReviewTask => ({
    ID: 'r1',
    ContentItemID: 'c1',
    DraftTitle: 'T',
    DraftBody: '',
    Status: 'pending',
    ...over,
  });

  it('keeps pending only and marks action approve', () => {
    const lane = buildPublishLane([mk({}), mk({ ID: 'r2', Status: 'approved' })], 'now', false);
    expect(lane.count).toBe(1);
    expect(lane.items[0]).toMatchObject({ id: 'review:r1', lane: 'publish', action: 'approve' });
  });

  it('falls back to body slice / ID for summary and "machine" for who', () => {
    const lane = buildPublishLane([mk({ DraftTitle: '', DraftBody: 'b'.repeat(80) })], 'now', false);
    expect(lane.items[0].summary).toHaveLength(60);
    expect(lane.items[0].who).toBe('machine');
  });
});

describe('buildListingLane', () => {
  const mk = (over: Partial<Opportunity>): Opportunity => ({
    ID: 'o1',
    Title: 'Opp',
    Summary: '',
    Audience: 'ec',
    Status: 'new',
    Score: 0,
    ...over,
  });

  it('accepts pending/new/open/empty statuses', () => {
    const lane = buildListingLane(
      [mk({}), mk({ ID: 'o2', Status: 'closed' }), mk({ ID: 'o3', Status: '' })],
      'now',
      false,
    );
    expect(lane.count).toBe(2);
    expect(lane.items[0]).toMatchObject({ id: 'opp:o1', action: 'ack' });
  });

  it('appends score to summary only when non-zero', () => {
    const withScore = buildListingLane([mk({ Score: 0.85 })], 'now', false);
    expect(withScore.items[0].summary).toBe('Opp · 0.85');
    const noScore = buildListingLane([mk({})], 'now', false);
    expect(noScore.items[0].summary).toBe('Opp');
  });
});
