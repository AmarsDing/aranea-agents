/**
 * Re-export barrel — WS transport has been lifted to the shared
 * realtime/ directory. This file re-exports for backward compatibility.
 *
 * New code should import from "realtime/ws-transport" directly.
 */

export { createWsTransport } from '../../realtime/ws-transport';

export type { WsTransportOptions, WsTransport } from '../../realtime/ws-transport';
