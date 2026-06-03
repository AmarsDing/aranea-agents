import { describe, expect, it } from 'vitest';
import type { Envelope } from '../envelope';
import { shouldChannelInboundCompleteToast } from '../channelInboundSession';
import { SESSION_RUN_STATUS } from '../sessionRunStatus';

function env(partial: Partial<Envelope>): Envelope {
  return {
    id: 'e1',
    type: 'run_status',
    author: 'test',
    session_id: 'sess-1',
    timestamp: '',
    version: 1,
    ...partial,
  };
}

describe('shouldChannelInboundCompleteToast', () => {
  it('toasts on runner_completion', () => {
    expect(shouldChannelInboundCompleteToast(env({ type: 'runner_completion' }))).toBe(true);
  });

  it('toasts on channel revision completed (M55 primary path)', () => {
    expect(
      shouldChannelInboundCompleteToast(
        env({
          source: 'channel',
          metadata: { status: SESSION_RUN_STATUS.COMPLETED },
        }),
      ),
    ).toBe(true);
  });

  it('does not toast on generic run_status completed without channel source', () => {
    expect(shouldChannelInboundCompleteToast(env({ metadata: { status: SESSION_RUN_STATUS.COMPLETED } }))).toBe(false);
  });

  it('toasts on failed/cancelled channel turns', () => {
    expect(shouldChannelInboundCompleteToast(env({ metadata: { status: SESSION_RUN_STATUS.FAILED } }))).toBe(true);
  });
});
