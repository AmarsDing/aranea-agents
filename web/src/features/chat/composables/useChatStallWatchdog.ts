import { watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';

/**
 * 长运行停滞看门狗：runStatus 为 'running' 但 5 分钟无事件时，
 * 弹出「似乎没有进展，是否停止？」通知。通过包装 sender.touchRunActivity
 * 在每次 run 活动事件时重置计时器。
 */

// 附着在 touchRunActivity 包装函数上的守卫标记（F3，2026-10-19）：
// 同一 sender 被多个 watchdog 实例 patch 时，不叠加第二层包装——
// 旧实现的叠包会让第一层闭包继续重置其已失效实例的计时器。
const STALL_PATCHED = Symbol.for('aranea.stallWatchdogPatched');

type TouchRunActivity = { (eventSessionId?: string): void; [STALL_PATCHED]?: true };

export function useChatStallWatchdog(deps: {
  runStatus: { value: string };
  sender: { touchRunActivity: (eventSessionId?: string) => void };
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
          message: t('chat.runLongStallWarning'),
          actions: [
            {
              label: t('chat.stop'),
              color: 'negative',
              handler: () => deps.stopStreaming(),
            },
            {
              label: t('chat.wait'),
              color: 'grey',
              handler: () => {},
            },
          ],
          timeout: 15_000,
        });
      }, STALL_NOTIFY_TIMEOUT_MS);
    }
  }

  // Start/stop stall timer based on runStatus.
  // F3: immediate:true — 组合式挂载时 runStatus 已经是 'running'
  // （例如切换到进行中的会话）也必须立即启动计时器，而非等待下一次状态翻转。
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
    { immediate: true },
  );

  // Reset stall timer whenever we receive a run activity event
  const senderTouch = deps.sender.touchRunActivity as TouchRunActivity;
  if (!senderTouch[STALL_PATCHED]) {
    const origTouchRunActivity = senderTouch.bind(deps.sender);
    const wrapped: TouchRunActivity = (eventSessionId?: string) => {
      origTouchRunActivity(eventSessionId);
      if (stallNotified) {
        resetStallNotifyTimer();
      } else if (deps.runStatus.value === 'running' && stallNotifyTimer != null) {
        resetStallNotifyTimer();
      }
    };
    wrapped[STALL_PATCHED] = true;
    deps.sender.touchRunActivity = wrapped;
  }

  return { clearStallNotifyTimer };
}
