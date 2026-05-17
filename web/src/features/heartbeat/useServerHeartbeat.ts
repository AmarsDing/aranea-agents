import { ref } from "vue";
import { buildHealthWsUrl, getWsOrigin } from "./api";
import { useAuthStore } from "../../stores/auth";
import { Notify } from "quasar";

export type ServerHeartbeatOptions = {
  pingInterval?: number;
  pongTimeout?: number;
  reconnectBaseDelay?: number;
  reconnectMaxDelay?: number;
};

const DEFAULT_PING_INTERVAL = 15_000;
const DEFAULT_PONG_TIMEOUT = 45_000;
const DEFAULT_RECONNECT_BASE_DELAY = 1_000;
const DEFAULT_RECONNECT_MAX_DELAY = 30_000;

export type ServerHeartbeatState = {
  isAlive: boolean;
  lastPongAt: number | null;
  connected: boolean;
};

export function useServerHeartbeat(options?: ServerHeartbeatOptions) {
  const pingInterval = options?.pingInterval ?? DEFAULT_PING_INTERVAL;
  const pongTimeout = options?.pongTimeout ?? DEFAULT_PONG_TIMEOUT;
  const reconnectBaseDelay = options?.reconnectBaseDelay ?? DEFAULT_RECONNECT_BASE_DELAY;
  const reconnectMaxDelay = options?.reconnectMaxDelay ?? DEFAULT_RECONNECT_MAX_DELAY;

  const isAlive = ref(true);
  const lastPongAt = ref<number | null>(null);
  const connected = ref(false);

  let ws: WebSocket | null = null;
  let pingTimer: ReturnType<typeof setInterval> | null = null;
  let timeoutTimer: ReturnType<typeof setInterval> | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let reconnectAttempts = 0;
  let stopped = false;
  let shutdownReceived = false;

  function connect(): void {
    if (stopped) return;
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
      return;
    }

    const url = buildHealthWsUrl();
    try {
      ws = new WebSocket(url);
    } catch {
      scheduleReconnect();
      return;
    }

    ws.onopen = () => {
      connected.value = true;
      isAlive.value = true;
      reconnectAttempts = 0;
      lastPongAt.value = Date.now();
      startPing();
      startTimeoutCheck();
    };

    ws.onmessage = (ev: MessageEvent) => {
      try {
        const msg = JSON.parse(ev.data as string);
        if (msg.type === "pong" || msg.type === "connected") {
          lastPongAt.value = Date.now();
          isAlive.value = true;
        }
        if (msg.type === "server_shutdown") {
          shutdownReceived = true;
          handleTimeout();
        }
      } catch {
        // ignore
      }
    };

    ws.onclose = () => {
      connected.value = false;
      stopPing();
      stopTimeoutCheck();
      if (!shutdownReceived) {
        scheduleReconnect();
      }
    };

    ws.onerror = () => {
      // onclose will fire after onerror
    };
  }

  function disconnect(): void {
    stopped = true;
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    stopPing();
    stopTimeoutCheck();
    if (ws) {
      ws.onclose = null;
      ws.onerror = null;
      ws.onmessage = null;
      ws.close(1000, "heartbeat disconnect");
      ws = null;
    }
    connected.value = false;
  }

  function scheduleReconnect(): void {
    if (stopped || reconnectTimer) return;
    const delay = Math.min(reconnectBaseDelay * Math.pow(2, reconnectAttempts), reconnectMaxDelay);
    reconnectAttempts++;
    reconnectTimer = setTimeout(() => {
      reconnectTimer = null;
      connect();
    }, delay);
  }

  function startPing(): void {
    stopPing();
    pingTimer = setInterval(() => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ direction: "client_to_server", channel: "system", type: "ping" }));
      }
    }, pingInterval);
  }

  function stopPing(): void {
    if (pingTimer) {
      clearInterval(pingTimer);
      pingTimer = null;
    }
  }

  function startTimeoutCheck(): void {
    stopTimeoutCheck();
    timeoutTimer = setInterval(() => {
      if (!lastPongAt.value) {
        handleTimeout();
        return;
      }
      const elapsed = Date.now() - lastPongAt.value;
      if (elapsed > pongTimeout) {
        handleTimeout();
      }
    }, 5_000);
  }

  function stopTimeoutCheck(): void {
    if (timeoutTimer) {
      clearInterval(timeoutTimer);
      timeoutTimer = null;
    }
  }

  async function handleTimeout(): Promise<void> {
    isAlive.value = false;
    const auth = useAuthStore();
    auth.user = null;
    auth.sessionChecked = true;

    Notify.create({
      type: "warning",
      message: shutdownReceived ? "服务器已关闭，请重新登录" : "服务器连接超时，请重新登录",
      timeout: 0,
      actions: [{ label: "重新登录", color: "white", handler: () => {} }],
    });

    const { default: router } = await import("../../router");
    const current = router.currentRoute.value;
    if (current.path !== "/login") {
      router.replace({ path: "/login", query: { redirect: current.fullPath } });
    }
  }

  connect();

  return {
    isAlive,
    lastPongAt,
    connected,
    connect,
    disconnect,
  };
}
