import type { Envelope } from './envelope';
import { envelopeSource } from './inboundSyncEnvelope';
import { SESSION_RUN_STATUS } from './sessionRunStatus';

export type ChannelFocusOptions = {
  /** Skip full message reload when auto-focusing during a live channel turn (DECO-R-P2-01 / BL-04). */
  skipMessageReload?: boolean;
};

export function shouldScheduleChannelFocus(input: {
  channelInbound: boolean;
  channelAgentId: string;
  focusTrigger: boolean;
  isChatRoute: boolean;
  isViewingSession: boolean;
  shouldAutoFocus: boolean;
  hasFocusHandler: boolean;
}): boolean {
  return (
    input.channelInbound &&
    !!input.channelAgentId &&
    input.focusTrigger &&
    input.isChatRoute &&
    !input.isViewingSession &&
    input.shouldAutoFocus &&
    input.hasFocusHandler
  );
}

/** RUNNING auto-focus should not reload messages and wipe ephemeral stream rows. */
export function shouldSkipMessageReloadOnChannelFocus(runStatus: string): boolean {
  return runStatus === SESSION_RUN_STATUS.RUNNING;
}

export function isStreamEnvelopeType(env: Envelope): boolean {
  return (
    env.type === 'text_delta' || env.type === 'text_done' || env.type === 'tool_call' || env.type === 'tool_result'
  );
}

/** DECO-R-P1-01: Global hub patches stream only for channel inbound (session WS owns web turns). */
export function shouldGlobalHubHandleStream(
  channelInbound: boolean,
  entityKind: 'agent' | 'team',
  env: Envelope,
): boolean {
  return channelInbound && entityKind === 'agent' && isStreamEnvelopeType(env);
}

/** DECO-R-P1-01: Session WS must not duplicate channel stream patches. */
export function shouldSessionWsSkipEnvelope(env: Envelope): boolean {
  return envelopeSource(env) === 'channel';
}

/** DECO-R-P1-02: Global hub finalizeTurn for channel or background sessions; web current session uses session WS reload. */
export function shouldGlobalHubFinalizeTurn(
  channelInbound: boolean,
  isCurrent: boolean,
  turnComplete: boolean,
): boolean {
  if (!turnComplete) return false;
  return channelInbound || !isCurrent;
}
