import { describe, expect, it } from 'vitest';
import { ChatApiError, isChatQueueFullError } from '../api';

describe('isChatQueueFullError', () => {
  it('matches kratos reason and human message', () => {
    expect(isChatQueueFullError(new Error('[CHAT_QUEUE_FULL/RATE_LIMITED] pending queue is full'))).toBe(true);
    expect(isChatQueueFullError(new Error('pending queue is full for this session'))).toBe(true);
    expect(isChatQueueFullError(new Error('排队消息已满，请稍后再试'))).toBe(true);
    expect(isChatQueueFullError(new Error('network down'))).toBe(false);
  });

  it('matches wrapped axios 429 on enqueue', () => {
    const ax = {
      response: { status: 429, data: { reason: 'CHAT_QUEUE_FULL_RATE_LIMITED' } },
      config: { url: '/v1/chat/enqueue' },
    };
    expect(isChatQueueFullError(new ChatApiError('pending queue is full for this session', ax))).toBe(true);
  });
});
