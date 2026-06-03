/**
 * Re-export barrel — the EnvelopeDispatcher has been lifted to the shared
 * realtime/ directory. This file re-exports for backward compatibility.
 *
 * New code should import from "realtime/dispatcher" directly.
 */

export { EnvelopeDispatcher, matchFilterKey } from '../../realtime/dispatcher';

export type { EnvelopeHandler, DispatcherFilter } from '../../realtime/dispatcher';
