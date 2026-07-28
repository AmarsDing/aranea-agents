/**
 * P2 (mobile): pure logic for turning blocking steps (confirm / clarify)
 * into local notification descriptors. Kept framework-free for testability;
 * the Tauri side-effect wrapper lives in services/localNotification.ts and
 * the store watcher in composables/useBlockingStepNotifications.ts.
 */
import type { Step } from '../v2Types';
import { i18n } from '../../i18n';

export type BlockingNotification = {
  stepId: string;
  sessionId: string;
  kind: 'confirm' | 'clarify';
  title: string;
  body: string;
};

const BODY_MAX = 80;

function truncate(text: string): string {
  const t = text.trim();
  return t.length > BODY_MAX ? `${t.slice(0, BODY_MAX)}…` : t;
}

/**
 * Returns notifications for steps that are currently blocked on user input
 * and have not been notified yet. `alreadyNotified` holds step IDs notified
 * in prior passes (reconnect replays re-send running steps — dedup by ID).
 */
export function collectBlockingSteps(
  steps: Iterable<Step>,
  alreadyNotified: ReadonlySet<string>,
): BlockingNotification[] {
  const out: BlockingNotification[] = [];
  for (const step of steps) {
    if (step.Status !== 'running') continue;
    if (step.Kind !== 'confirm' && step.Kind !== 'clarify') continue;
    if (alreadyNotified.has(step.ID)) continue;
    out.push({
      stepId: step.ID,
      sessionId: step.SessionID,
      kind: step.Kind,
      title: i18n.global.t(step.Kind === 'confirm' ? 'mobile.notifyConfirmTitle' : 'mobile.notifyClarifyTitle'),
      body:
        truncate(step.Content) ||
        i18n.global.t(step.Kind === 'confirm' ? 'mobile.notifyConfirmBody' : 'mobile.notifyClarifyBody'),
    });
  }
  return out;
}

/** Deep-link route embedded in the notification; tap navigates to the chat. */
export function buildNotificationRoute(n: BlockingNotification): string {
  if (!n.sessionId) return '/mobile/sessions';
  return `/mobile/chat?session=${encodeURIComponent(n.sessionId)}`;
}
