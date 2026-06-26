import { describe, expect, it } from 'vitest';
import { SESSION_RUN_STATUS } from '../sessionRunStatus';
import {
  shouldGlobalHubFinalizeTurn,
  shouldScheduleChannelFocus,
  shouldSkipMessageReloadOnChannelFocus,
} from '../inboundSyncRouting';

describe('inboundSyncRouting (DECO-R-P1)', () => {
  it('global hub finalizeTurn for channel or background sessions', () => {
    expect(shouldGlobalHubFinalizeTurn(true, true, true)).toBe(true);
    expect(shouldGlobalHubFinalizeTurn(true, false, true)).toBe(true);
    expect(shouldGlobalHubFinalizeTurn(false, false, true)).toBe(true);
    expect(shouldGlobalHubFinalizeTurn(false, true, true)).toBe(false);
    expect(shouldGlobalHubFinalizeTurn(false, true, false)).toBe(false);
  });

  it('schedules channel focus only when auto-focus preconditions hold', () => {
    expect(
      shouldScheduleChannelFocus({
        channelInbound: true,
        channelAgentId: 'agent-1',
        focusTrigger: true,
        isChatRoute: true,
        isViewingSession: false,
        shouldAutoFocus: true,
        hasFocusHandler: true,
      }),
    ).toBe(true);
    expect(
      shouldScheduleChannelFocus({
        channelInbound: true,
        channelAgentId: 'agent-1',
        focusTrigger: true,
        isChatRoute: true,
        isViewingSession: true,
        shouldAutoFocus: true,
        hasFocusHandler: true,
      }),
    ).toBe(false);
  });

  it('skips message reload on RUNNING channel focus (DECO-R-P2-01)', () => {
    expect(shouldSkipMessageReloadOnChannelFocus(SESSION_RUN_STATUS.RUNNING)).toBe(true);
    expect(shouldSkipMessageReloadOnChannelFocus(SESSION_RUN_STATUS.COMPLETED)).toBe(false);
  });
});
