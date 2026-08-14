import { watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';

/**
 * 长运行停滞看门狗：runStatus 为 'running' 但 5 分钟无事件时，
 * 弹出「似乎没有进展，是否停止？」通知。通过包装 sender.touchRunActivity
 * 在每次 run 活动事件时重置计时器。
 */
export function useChatStallWatchdog(deps: {
  runStatus: { value: string };
  sender: { touchRunActivity: () => void };
  stopStreaming: () => void;
}) {
  const { t } = useI18n();
  const $q = useQuasar();

  const STALL_NOTIFY_TIMEOUT_MS = 5 * 60 * 1000; // 5 minutes
  let stallNotifyTimer: ReturnType<typeof setTimeout> | null = null;
  let stallNotified = false;

  function clearStallNotifyTimer() {
    if (stallNotifyTimer != null) {
      clearTimeout(stallNotifyTimer);
      stallNotifyTimer = null;
    }
  }

  function resetStallNotifyTimer() {
    clearStallNotifyTimer();
    stallNotified = false;
    if (deps.runStatus.value === 'running') {
      stallNotifyTimer = setTimeout(() => {
        stallNotified = true;
        $q.notify({
          type: 'warning',
          message: t('chat.runLongStallWarning', '似乎没有进展，是否停止？'),
          actions: [
            {
              label: t('chat.stop', '停止'),
              color: 'negative',
              handler: () => deps.stopStreaming(),
            },
            {
              label: t('chat.wait', '继续等待'),
              color: 'grey',
              handler: () => {},
            },
          ],
          timeout: 15_000,
        });
      }, STALL_NOTIFY_TIMEOUT_MS);
    }
  }

  // Start/stop stall timer based on runStatus
  watch(
    () => deps.runStatus.value,
    (newVal) => {
      if (newVal === 'running') {
        resetStallNotifyTimer();
      } else {
        clearStallNotifyTimer();
        stallNotified = false;
      }
    },
  );

  // Reset stall timer whenever we receive a run activity event
  const origTouchRunActivity = deps.sender.touchRunActivity.bind(deps.sender);
  deps.sender.touchRunActivity = () => {
    origTouchRunActivity();
    if (stallNotified) {
      resetStallNotifyTimer();
    } else if (deps.runStatus.value === 'running' && stallNotifyTimer != null) {
      resetStallNotifyTimer();
    }
  };

  return { clearStallNotifyTimer };
}
