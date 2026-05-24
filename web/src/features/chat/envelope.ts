/**
 * Re-export barrel — all envelope types have been lifted to the shared
 * realtime/ directory. This file re-exports them for backward compatibility
 * so that existing imports from "features/chat/envelope" continue to work.
 *
 * New code should import from "realtime/envelope" directly.
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
} from "../../realtime/envelope";
