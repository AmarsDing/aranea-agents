import { computed, watch } from 'vue';
import type { CompressStatus } from '../../session/types';
import { useChatSessionStore } from '../../../stores/chat/sessionStore';

/**
 * 压缩状态轮询：会话压缩进行中每 5s 拉一次状态；状态回到 'normal'
 * 并持续 10s 冷却后自动停止，避免无限空转占用网络。
 */
export function useChatCompressPolling(deps: { selectedSessionId: () => string | undefined }) {
  const sessionStore = useChatSessionStore();

  const compressStatus = computed<CompressStatus>(() => sessionStore.compressStatus);
  let compressPollTimer: ReturnType<typeof setInterval> | null = null;
  let compressNormalSince: number | null = null;
  const COMPRESS_POLL_INTERVAL_MS = 5_000;
  const COMPRESS_NORMAL_COOLDOWN_MS = 10_000;

  async function pollCompressStatus() {
    const sid = deps.selectedSessionId();
    if (!sid) {
      // No session selected (e.g. navigated away from chat): stop the timer
      // instead of spinning a no-op interval forever.
      stopCompressPolling();
      return;
    }
    await sessionStore.fetchCompressStatus(sid);
    // The stop-condition must be evaluated here, after every poll, rather
    // than in a watch on compressStatus: a watch only fires on value change,
    // so a steady 'normal' never re-triggers it and the cooldown check below
    // would never run again — polling continued forever (5s interval kept the
    // network busy, which starved browser-tool readiness on the chat page).
    if (sessionStore.compressStatus === 'normal') {
      if (!compressNormalSince) {
        compressNormalSince = Date.now();
      }
      if (Date.now() - compressNormalSince >= COMPRESS_NORMAL_COOLDOWN_MS) {
        stopCompressPolling();
      }
    } else {
      compressNormalSince = null;
    }
  }

  function startCompressPolling() {
    stopCompressPolling();
    void pollCompressStatus();
    compressPollTimer = setInterval(() => {
      void pollCompressStatus();
    }, COMPRESS_POLL_INTERVAL_MS);
  }

  function stopCompressPolling() {
    if (compressPollTimer) {
      clearInterval(compressPollTimer);
      compressPollTimer = null;
    }
    compressNormalSince = null;
  }

  // Restart polling if the status ever flips to non-normal while the timer
  // is stopped (defensive: today only pollCompressStatus updates the status,
  // but a future WS push path would need this to resume polling).
  watch(compressStatus, (status) => {
    if (status !== 'normal' && !compressPollTimer) {
      startCompressPolling();
    }
  });

  return { compressStatus, startCompressPolling, stopCompressPolling };
}
