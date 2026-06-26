import type { Message, RunStatusValue } from './types';

export type ConversationSource = 'web' | 'ws' | 'channel' | 'cron' | 'a2a' | 'durable';

export type ConversationTargetType = 'agent' | 'team';

export type ConversationTarget = {
  type: ConversationTargetType;
  id: string;
  key?: string;
  name?: string;
  source?: ConversationSource;
};

export type ConversationSession = {
  id: string;
  target: ConversationTarget;
  title: string;
  unreadCount: number;
  pinnedAt?: string;
  source?: ConversationSource;
  lastTurn?: ConversationTurnSummary;
};

export type ConversationTurnStatus =
  | 'queued'
  | 'running'
  | 'awaiting_user'
  | 'background'
  | 'completed'
  | 'failed'
  | 'cancelled';

export type DeliveryStatus = 'not_required' | 'pending' | 'sending' | 'delivered' | 'failed' | 'skipped';

export type DeliveryTarget = {
  kind: 'chat' | 'channel' | 'monitor' | string;
  channelId?: string;
  platform?: string;
  recipientId?: string;
  status: DeliveryStatus;
  error?: string;
  updatedAt?: string;
};

export type ConversationTurnSummary = {
  id: string;
  sessionId: string;
  runId?: string;
  status: ConversationTurnStatus;
  source: ConversationSource;
  revision: number;
  deliveryTargets: DeliveryTarget[];
  updatedAt?: string;
};

export type ConversationTimelineItem = {
  kind: 'message' | 'event';
  turnId?: string;
  message?: Message;
};

export function runStatusToTurnStatus(status: RunStatusValue | string): ConversationTurnStatus | undefined {
  switch (status) {
    case 'pending':
      return 'queued';
    case 'running':
    case 'sync':
      return 'running';
    case 'escalating':
    case 'durable':
      return 'background';
    case 'awaiting_user':
      return 'awaiting_user';
    case 'completed':
      return 'completed';
    case 'failed':
      return 'failed';
    case 'cancelled':
      return 'cancelled';
    default:
      return undefined;
  }
}

export function deliveryStatusFromChannelStatus(status: string): DeliveryStatus | undefined {
  switch (status.trim().toLowerCase()) {
    case 'queued':
    case 'pending':
      return 'pending';
    case 'sending':
    case 'streaming':
    case 'streamed':
      return 'sending';
    case 'sent':
    case 'delivered':
    case 'ok':
    case 'success':
      return 'delivered';
    case 'failed':
    case 'error':
    case 'timeout':
      return 'failed';
    case 'skipped':
    case 'skipped_duplicate':
    case 'skipped_access':
    case 'skipped_empty':
      return 'skipped';
    default:
      return undefined;
  }
}
