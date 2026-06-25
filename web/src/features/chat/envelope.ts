/**
 * Re-export barrel — all envelope types have been lifted to the shared
 * realtime/ directory. This file re-exports them for backward compatibility
 * so that existing imports from "features/chat/envelope" continue to work.
 *
 * New code should import from "realtime/envelope" directly.
 *
 * @deprecated The Activity-First architecture replaces envelope-based chat
 * events with ActivityEvent. Chat code should consume ActivityEvent via
 * useActivityTimeline instead of envelope types. See ADR-02 §遗留项.
 */

export type {
  EnvelopeType,
  EnvelopeContent,
  EnvelopeToolCall,
  EnvelopeStateDelta,
  EnvelopeTransfer,
  EnvelopeError,
  EnvelopeUsage,
  EnvelopeActions,
  EnvelopeTrace,
  Envelope,
  WsDownstream,
  WsUpstream,
} from '../../realtime/envelope';
