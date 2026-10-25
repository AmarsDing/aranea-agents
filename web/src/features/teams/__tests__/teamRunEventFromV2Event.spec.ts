/**
 * teamRunEventFromV2Event：v2 信封 → TeamRunEvent 映射契约（D4-5）。
 *
 *  ① team_run.cancelled → type=team_run_cancelled、run.status=cancelled
 *  ② started/completed/failed 回归基线（既有三分支不错位）
 */
import { describe, it, expect } from 'vitest';
import { teamRunEventFromV2Event } from '../teamRunEventFromV2Event';
import type { V2WsEnvelope, TeamRun as V2TeamRun } from '../../chat/v2Types';

function mkV2TeamRun(over: Partial<V2TeamRun> = {}): V2TeamRun {
  return {
    ID: 'run-1',
    TeamStageID: 'ts-1',
    TaskID: 't-1',
    SessionID: 'sess-1',
    SpiritSessionID: 'sp-1',
    DagNodeID: '',
    DependsOn: [],
    Status: 'running',
    StartedAt: '2026-10-21T00:00:00Z',
    CompletedAt: null,
    Seq: 1,
    Version: 1,
    Error: '',
    ...over,
  };
}

function env(kind: V2WsEnvelope['kind'], payload: unknown): V2WsEnvelope {
  return { type: 'v2_event', kind, session_id: 'sess-1', payload } as V2WsEnvelope;
}

describe('teamRunEventFromV2Event', () => {
  it('maps team_run.cancelled to team_run_cancelled with run.status cancelled', () => {
    const ev = teamRunEventFromV2Event(
      env('team_run.cancelled', { TeamRun: mkV2TeamRun({ Status: 'cancelled', Version: 2 }) }),
      'team-1',
    );
    expect(ev).not.toBeNull();
    expect(ev?.type).toBe('team_run_cancelled');
    expect(ev?.run?.status).toBe('cancelled');
    expect(ev?.run_id).toBe('run-1');
    expect(ev?.team_id).toBe('team-1');
    expect(ev?.session_id).toBe('sess-1');
  });

  it('regression: started/completed/failed branches keep their legacy types', () => {
    expect(
      teamRunEventFromV2Event(env('team_run.started', { TeamRun: mkV2TeamRun({ Status: 'running' }) }), 'team-1')?.type,
    ).toBe('team_run_started');
    const finished = teamRunEventFromV2Event(
      env('team_run.completed', { TeamRun: mkV2TeamRun({ Status: 'completed' }) }),
      'team-1',
    );
    expect(finished?.type).toBe('team_run_finished');
    expect(finished?.run?.status).toBe('completed');
    const failed = teamRunEventFromV2Event(
      env('team_run.failed', { TeamRun: mkV2TeamRun({ Status: 'failed' }) }),
      'team-1',
    );
    expect(failed?.type).toBe('team_run_failed');
    expect(failed?.run?.status).toBe('failed');
  });

  it('returns null when the cancelled payload lacks TeamRun', () => {
    expect(teamRunEventFromV2Event(env('team_run.cancelled', {}), 'team-1')).toBeNull();
  });
});
