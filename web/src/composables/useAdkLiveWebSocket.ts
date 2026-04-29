import { onUnmounted, ref } from "vue";
import { buildAdkLiveUrl } from "../config/runtime";
import { AdkLiveWebSocket, type AdkLiveEvent, type AdkLiveRequest } from "../api/adk";

/**
 * 音频/视频 Live 会话：在组件内维护单一 WebSocket 与连接状态。
 */
export function useAdkLiveWebSocket() {
  const open = ref(false);
  const lastText = ref<string | null>(null);
  const lastError = ref<Event | null>(null);
  const client = new AdkLiveWebSocket({
    onMessage: (data) => {
      lastText.value = data;
    },
    onError: (e) => {
      lastError.value = e;
    },
    onOpen: () => {
      open.value = true;
    },
    onClose: () => {
      open.value = false;
    }
  });

  onUnmounted(() => {
    client.close();
  });

  function connect(params: { appName: string; userId: string; sessionId: string }) {
    const url = buildAdkLiveUrl(params);
    client.connect(url);
  }

  function sendMessage(payload: AdkLiveRequest) {
    client.sendMessage(payload);
  }

  function parseLastEvent(): AdkLiveEvent | null {
    if (!lastText.value) {
      return null;
    }
    return AdkLiveWebSocket.parseEvent(lastText.value);
  }

  return {
    open,
    lastText,
    lastError,
    connected: () => client.connected,
    connect,
    sendMessage,
    parseLastEvent,
    close: () => client.close()
  };
}
