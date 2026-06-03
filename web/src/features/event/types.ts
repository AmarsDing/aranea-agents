import type { Envelope } from '../../realtime/envelope';

export type ListSessionEventsParams = {
  sessionId: string;
  since?: string;
  until?: string;
  type?: string;
  limit?: number;
  offset?: number;
};

export type ListSessionEventsResult = {
  items: Envelope[];
  total: number;
};
