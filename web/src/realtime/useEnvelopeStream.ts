/**
 * Isolated compat alias — production features must not import this module.
 *
 * Use:
 *   - createV2EventStream / useV2EventStream  (graph / teams / knowledge / orchestration / chat v2)
 *   - createMonitorStream                     (monitor_event)
 *   - createChatStream / createTeamStream     (chat send path)
 *
 * The activity_event branch has been removed. This file only re-exports the
 * typed session stream so leftover tests can keep compiling.
 */
export {
  createWsSessionStream as createEnvelopeStream,
  type WsSessionStream as UseEnvelopeStreamReturn,
  type WsSessionStreamOptions as UseEnvelopeStreamOptions,
} from './createWsSessionStream';

export type {
  GraphNodeState,
  GraphExecutionState,
  GraphStreamInterrupt,
  GraphStreamExecutionSummary,
} from './graphState';
