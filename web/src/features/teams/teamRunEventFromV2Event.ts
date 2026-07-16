/**
 * Map typed v2 WS envelopes (team_run.* / member_session.* / system.notice)
 * onto the TeamRunEvent shape used by Teams/Monitor UI.
 */
import type { V2WsEnvelope, TeamRun as V2TeamRun, MemberSession, SystemNoticeEventPayload } from '../chat/v2Types';
import type { TeamRun, TeamRunEvent, TeamRunStep } from './types';
import { activityEventFromSystemNotice } from '../../realtime/systemNoticeAdapter';
import { teamRunEventFromActivityEvent } from './teamRunEventFromActivityEvent';

function asRecord(v: unknown): Record<string, unknown> {
  return v && typeof v === 'object' ? (v as Record<string, unknown>) : {};
}

function wireTeamRunFromV2(tr: V2TeamRun, defaultTeamID: string): TeamRun {
  return {
    id: tr.ID ?? '',
    team_id: defaultTeamID,
    session_id: tr.SessionID ?? '',
    message_id: '',
    mode: '',
    status: String(tr.Status ?? ''),
    input_preview: '',
    output_preview: '',
    token_in: 0,
    token_out: 0,
    cost_micro_usd: 0,
    duration_ms: 0,
    error_message: tr.Error ?? '',
    topology_json: '',
    graph_execution_id: '',
    started_at: tr.StartedAt ?? '',
    finished_at: tr.CompletedAt ?? '',
    created_at: tr.StartedAt ?? '',
    updated_at: tr.CompletedAt ?? tr.StartedAt ?? '',
  };
}

function stepFromMemberSession(ms: MemberSession, defaultTeamID: string): TeamRunStep {
  return {
    id: ms.ID ?? '',
    run_id: ms.TeamRunID ?? '',
    team_id: defaultTeamID,
    agent_id: '',
    agent_key: ms.AgentKey ?? '',
    agent_name: ms.AgentName ?? '',
    role: '',
    sort_order: 0,
    status: String(ms.Status ?? ''),
    input_preview: '',
    output_preview: '',
    token_in: 0,
    token_out: 0,
    cost_micro_usd: 0,
    duration_ms: 0,
    error_message: ms.Error ?? '',
    started_at: ms.StartedAt ?? '',
    finished_at: ms.FinishedAt ?? '',
    created_at: ms.StartedAt ?? '',
  };
}

/**
 * Convert a v2 envelope into a TeamRunEvent when it carries team-run telemetry.
 * Returns null for unrelated kinds.
 */
export function teamRunEventFromV2Event(envelope: V2WsEnvelope, defaultTeamID = ''): TeamRunEvent | null {
  const sessionId = String(envelope.session_id ?? '').trim();
  const payload = envelope.payload as unknown as Record<string, unknown>;

  switch (envelope.kind) {
    case 'team_run.started': {
      const tr = payload.TeamRun as V2TeamRun | undefined;
      if (!tr) return null;
      const run = wireTeamRunFromV2(tr, defaultTeamID);
      return {
        type: 'team_run_started',
        team_id: defaultTeamID || run.team_id,
        run_id: run.id,
        session_id: sessionId || run.session_id,
        run,
        payload: asRecord(tr),
      };
    }
    case 'team_run.completed': {
      const tr = payload.TeamRun as V2TeamRun | undefined;
      if (!tr) return null;
      const run = wireTeamRunFromV2(tr, defaultTeamID);
      return {
        type: 'team_run_finished',
        team_id: defaultTeamID || run.team_id,
        run_id: run.id,
        session_id: sessionId || run.session_id,
        run,
        payload: asRecord(tr),
      };
    }
    case 'team_run.failed': {
      const tr = payload.TeamRun as V2TeamRun | undefined;
      if (!tr) return null;
      const run = wireTeamRunFromV2(tr, defaultTeamID);
      return {
        type: 'team_run_failed',
        team_id: defaultTeamID || run.team_id,
        run_id: run.id,
        session_id: sessionId || run.session_id,
        run,
        payload: asRecord(tr),
      };
    }
    case 'member_session.created':
    case 'member_session.updated': {
      const ms = payload.MemberSession as MemberSession | undefined;
      if (!ms) return null;
      const status = String(ms.Status ?? '');
      const step = stepFromMemberSession(ms, defaultTeamID);
      const started = status === 'running' || status === 'pending' || envelope.kind === 'member_session.created';
      const finished = status === 'completed' || status === 'failed' || status === 'cancelled';
      if (!started && !finished) {
        return {
          type: 'member_session_updated',
          team_id: defaultTeamID,
          run_id: ms.TeamRunID ?? '',
          session_id: sessionId || ms.SpiritSessionID || ms.SessionID,
          step,
          payload: asRecord(ms),
        };
      }
      return {
        type: finished ? 'team_step_finished' : 'team_step_started',
        team_id: defaultTeamID,
        run_id: ms.TeamRunID ?? '',
        session_id: sessionId || ms.SpiritSessionID || ms.SessionID,
        step,
        payload: asRecord(ms),
      };
    }
    case 'system.notice': {
      // Reuse ActivityEvent mapper for intent_pass / team_summary / etc.
      const adapted = activityEventFromSystemNotice(envelope, envelope.payload as SystemNoticeEventPayload);
      return teamRunEventFromActivityEvent(adapted, defaultTeamID);
    }
    default:
      return null;
  }
}
