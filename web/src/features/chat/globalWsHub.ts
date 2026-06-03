/**
 * Re-export barrel — GlobalWsHub has been lifted to the shared
 * realtime/ directory. This file re-exports for backward compatibility.
 *
 * New code should import from "realtime/globalWsHub" directly.
 */

export {
  shouldUseGlobalWsHub,
  acquireGlobalWsConsumer,
  releaseGlobalWsConsumer,
  globalWsConsumerSubscribe,
  globalWsConsumerUnsubscribe,
  globalWsConsumerEnableLog,
  globalWsHubConnected,
} from '../../realtime/globalWsHub';

export type { GlobalWsConsumer } from '../../realtime/globalWsHub';
