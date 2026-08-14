/**
 * Reverse-map v2 system.notice → synthetic ActivityEvent for in-process
 * mappers that still speak the ActivityEvent contract (e.g. team-run
 * observatory). Not used by WS transport.
 */
import type { Activity, ActivityEvent, ActivityEventType, ActivityKind, ActivityStatus } from './activityEvent';
import type { SystemNoticeEventPayload, V2WsEnvelope } from '../features/chat/v2Types';

const TEAM_NOTICE_TYPES = new Set([
  'started',
  'finished',
  'failed',
  'step_started',
  'step_finished',
  'summary',
  'team_run_started',
  'team_run_finished',
  'team_run_failed',
  'team_step_started',
  'team_step_finished',
  'team_summary',
  'team_stage_assembled',
  'team_stage_completed',
  'intent_pass',
  'transfer',
]);

function asRecord(v: unknown): Record<string, unknown> {
  return v && typeof v === 'object' ? (v as Record<string, unknown>) : {};
}

function str(v: unknown, fallback = ''): string {
  return v == null ? fallback : String(v);
}

/**
 * Special-case remaps where NoticeType ≠ the historical activity.stage /
 * activity.kind pair that consumers filter on.
 */
function resolveKindAndStage(noticeType: string, meta: Record<string, unknown>): { kind: ActivityKind; stage: string } {
  const metaKind = str(meta.activity_kind);
  if (noticeType === 'checkpoint' || meta.interrupt_key != null) {
    return { kind: 'session', stage: 'checkpoint' };
  }
  if (noticeType === 'graph_task_status') {
    return { kind: (metaKind as ActivityKind) || 'graph_stage', stage: 'task_status' };
  }
  if (noticeType === 'team_summary' || noticeType === 'summary') {
    return { kind: 'team_stage', stage: 'summary' };
  }
  if (noticeType === 'intent_pass' || noticeType === 'transfer') {
    return { kind: 'notice', stage: noticeType };
  }
  if (TEAM_NOTICE_TYPES.has(noticeType) && (!metaKind || metaKind === 'team_stage' || metaKind === 'notice')) {
    if (noticeType.startsWith('team_stage_') || noticeType === 'intent_pass' || noticeType === 'transfer') {
      return {
        kind: noticeType === 'intent_pass' || noticeType === 'transfer' ? 'notice' : 'team_stage',
        stage: noticeType,
      };
    }
    return { kind: (metaKind as ActivityKind) || 'team_stage', stage: noticeType };
  }
  if (metaKind) {
    return { kind: metaKind as ActivityKind, stage: noticeType };
  }
  // Default: native system notices (knowledge_ingest, orchestration_status, …)
  return { kind: 'notice', stage: noticeType || 'notice' };
}

function emptyActivity(partial: Partial<Activity>): Activity {
  return {
    id: '',
    kind: 'notice',
    status: 'running',
    session_id: '',
    turn_id: '',
    parent_activity_id: '',
    timestamp: '',
    duration_ms: 0,
    seq: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    content: '',
    reasoning: '',
    tool_name: '',
    tool_category: 'other',
    tool_call_id: '',
    tool_arguments: '',
    tool_result: '',
    tool_duration_ms: 0,
    tool_error_code: '',
    stage: '',
    child_board_id: '',
    spirit_session_id: '',
    team_id: '',
    dag_node_id: '',
    depends_on: [],
    agent_key: '',
    agent_name: '',
    collapsed: false,
    label: '',
    meta: {},
    ...partial,
  };
}

/** Build a synthetic ActivityEvent from a system.notice payload. */
export function activityEventFromSystemNotice(
  envelope: Pick<V2WsEnvelope, 'session_id'>,
  payload: SystemNoticeEventPayload,
): ActivityEvent {
  const meta = { ...asRecord(payload.Meta) };
  const noticeType = str(payload.NoticeType);
  const { kind, stage } = resolveKindAndStage(noticeType, meta);
  const eventType = (str(meta.activity_event, 'updated') || 'updated') as ActivityEventType;
  const status = (str(meta.activity_status, 'running') || 'running') as ActivityStatus;
  const sessionId = str(envelope.session_id) || str(meta.session_id);

  // Drop adapter bookkeeping keys from consumer-facing meta (optional cleanup).
  // Keep them — consumers that check activity_kind are rare; graph filters use filter_key.

  return {
    event: eventType,
    domain: 'system',
    activity: emptyActivity({
      id: str(meta.activity_id) || str(meta.id) || `notice:${noticeType}:${sessionId}`,
      kind,
      status,
      session_id: sessionId,
      spirit_session_id: sessionId,
      content: str(payload.Message),
      stage,
      team_id: str(meta.team_id),
      agent_key: str(meta.agent_key),
      agent_name: str(meta.agent_name),
      dag_node_id: str(meta.dag_node_id),
      meta,
    }),
  };
}

/** True when a v2 envelope is a system.notice that can be adapted. */
export function isSystemNoticeEnvelope(envelope: V2WsEnvelope): boolean {
  return envelope.kind === 'system.notice';
}
