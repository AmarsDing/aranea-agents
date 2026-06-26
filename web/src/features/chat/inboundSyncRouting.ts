/**
 * Inbound sync routing helpers for ActivityEvent consumption. Chat code
 * routes ActivityEvent via useChatWorkspace.handleActivityEvent.
 */

import type { ActivityEvent } from '../../realtime/activityEvent';
import { activitySource } from './inboundSyncEnvelope';
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

/** DECO-R-P1-02: Global hub finalizeTurn for channel or background sessions; web current session uses session WS reload. */
export function shouldGlobalHubFinalizeTurn(
  channelInbound: boolean,
  isCurrent: boolean,
  turnComplete: boolean,
): boolean {
  if (!turnComplete) return false;
  return channelInbound || !isCurrent;
}

// ── ActivityEvent-based helpers (new) ───────────────────────────────────
// The backend now sends ALL chat/system events as ActivityEvent on the WS
// chat channel. These helpers mirror the Envelope-based ones above but
// consume ActivityEvent directly.

/**
 * Detect a streaming chat-rendering ActivityEvent
 * (kind=task AND event=streaming) — these events drive the Activity timeline
 * directly and are NOT processed by the inbound sync pipeline.
 */
export function isStreamActivityEvent(ev: ActivityEvent): boolean {
  return ev.activity.kind === 'task' && ev.event === 'streaming';
}

/**
 * Detect a tool-action ActivityEvent (kind=action) — these are also
 * chat-rendering events that drive the Activity timeline directly.
 */
export function isToolActionActivityEvent(ev: ActivityEvent): boolean {
  return ev.activity.kind === 'action';
}

/** DECO-R-P1-01: Session WS must not duplicate channel stream patches. */
export function shouldSessionWsSkipActivity(ev: ActivityEvent): boolean {
  return activitySource(ev) === 'channel';
}

/** DECO-R-P1-02: Global hub finalizeTurn for channel or background sessions; web current session uses session WS reload. */
export function shouldGlobalHubFinalizeTurnActivity(
  channelInbound: boolean,
  isCurrent: boolean,
  turnComplete: boolean,
): boolean {
  return shouldGlobalHubFinalizeTurn(channelInbound, isCurrent, turnComplete);
}
