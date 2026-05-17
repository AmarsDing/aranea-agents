/**
 * Composable that wraps the singleton `wsClient` for Vue components.
 *
 * Usage:
 * ```ts
 * const { isConnected, reconnectCount } = useWS("my-channel", (msg) => {
 *   console.log("received", msg);
 * });
 * ```
 *
 * The composable connects on mount, delivers messages filtered to `channel`,
 * and cleans up the subscription on unmount.  If `channel` is omitted, all
 * messages are delivered (wildcard subscription).
 */

import { onMounted, onUnmounted, readonly, ref } from "vue";
import { wsClient } from "../services/wsClient";
import { Notify } from "quasar";

type MessageHandler = (data: unknown) => void;

export function useWS(channel?: string, onMessage?: MessageHandler) {
  const isConnected = ref(wsClient.isConnected);
  const reconnectCount = ref(wsClient.reconnectCount);

  let unsub: (() => void) | undefined;
  let pollInterval: ReturnType<typeof setInterval> | undefined;

  onMounted(() => {
    // Sync connection state via polling (lightweight, avoids extra EventEmitter).
    pollInterval = setInterval(() => {
      const wasConnected = isConnected.value;
      isConnected.value = wsClient.isConnected;
      reconnectCount.value = wsClient.reconnectCount;

      // Show reconnect notification when connection drops.
      if (wasConnected && !isConnected.value) {
        Notify.create({ type: "warning", message: "服务器连接断开，正在重连…", timeout: 3000 });
      }
    }, 2000);

    if (onMessage) {
      unsub = channel
        ? wsClient.subscribe(channel, onMessage)
        : wsClient.subscribeAll(onMessage);
    }
  });

  onUnmounted(() => {
    unsub?.();
    if (pollInterval !== undefined) clearInterval(pollInterval);
  });

  return {
    isConnected: readonly(isConnected),
    reconnectCount: readonly(reconnectCount)
  };
}
