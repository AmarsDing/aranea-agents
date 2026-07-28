/**
 * P2 (mobile): watches the chat activity store for steps blocked on user
 * input (confirm / clarify) and raises a local notification for each new
 * one. Notifications only fire while the app is backgrounded
 * (`document.hidden`); steps seen while the app is visible are recorded so
 * they are not re-notified when the user later backgrounds the app.
 */
import { watch } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import { buildNotificationRoute, collectBlockingSteps } from '../blockingStepNotification';
import { notifyLocal } from '../../../services/localNotification';

/** Dedup-set cap; oldest entries are evicted (insertion order) beyond it. */
const NOTIFIED_CAP = 500;

export function useBlockingStepNotifications(): void {
  const store = useChatActivityStore();
  const notified = new Set<string>();

  watch(
    // Confirm/clarify steps are created in the running state, so watching
    // the map size covers every new blocking step; streaming merges do not
    // change the size and skip the scan entirely.
    () => store.steps.size,
    () => {
      const fresh = collectBlockingSteps(store.steps.values(), notified);
      for (const n of fresh) {
        notified.add(n.stepId);
        if (document.hidden) {
          void notifyLocal({ title: n.title, body: n.body, route: buildNotificationRoute(n) });
        }
      }
      if (notified.size > NOTIFIED_CAP) {
        const it = notified.values();
        for (let i = 0; i < NOTIFIED_CAP / 2; i++) {
          const v = it.next();
          if (v.done) break;
          notified.delete(v.value);
        }
      }
    },
  );
}
