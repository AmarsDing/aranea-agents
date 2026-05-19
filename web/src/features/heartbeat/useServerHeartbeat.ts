import { ref } from "vue";
import { buildHealthWsUrl } from "./api";
import { useAuthStore } from "../../stores/auth";
import { Notify } from "quasar";
import { getBackendOrigin, isWsSameOriginAsPage, readAccessTokenCookie } from "../../config/runtime";

export type ServerHeartbeatOptions = {
  pingInterval?: number;
  pongTimeout?: number;
  reconnectBaseDelay?: number;
  reconnectMaxDelay?: number;
  initialConnectTimeout?: number;
};

const DEFAULT_PING_INTERVAL = 15_000;
const DEFAULT_PONG_TIMEOUT = 30_000;
const DEFAULT_RECONNECT_BASE_DELAY = 1_000;
const DEFAULT_RECONNECT_MAX_DELAY = 30_000;
const DEFAULT_INITIAL_CONNECT_TIMEOUT = 8_000;

export type ServerHeartbeatState = {
  isAlive: boolean;
  lastPongAt: number | null;
  connected: boolean;
};

const isAlive = ref(true);
const lastPongAt = ref<number | null>(null);
const connected = ref(false);
let heartbeatInstance: ReturnType<typeof createHeartbeat> | null = null;

export function useServerHeartbeat(options?: ServerHeartbeatOptions) {
  if (!heartbeatInstance) {
    heartbeatInstance = createHeartbeat(options);
  }
  return heartbeatInstance;
}

export function getServerHeartbeatState() {
  return { isAlive, lastPongAt, connected };
}

export async function checkBackendHealth(): Promise<boolean> {
  const origin = getBackendOrigin();
  const baseUrl = origin ? `${origin}/healthz` : "/healthz";
  try {
    const resp = await fetch(baseUrl, { method: "GET", cache: "no-store", signal: AbortSignal.timeout(5000) });
    return resp.ok;
  } catch {
    return false;
  }
}

function createHeartbeat(options?: ServerHeartbeatOptions) {
  const pingInterval = options?.pingInterval ?? DEFAULT_PING_INTERVAL;
  const pongTimeout = options?.pongTimeout ?? DEFAULT_PONG_TIMEOUT;
  const reconnectBaseDelay = options?.reconnectBaseDelay ?? DEFAULT_RECONNECT_BASE_DELAY;
  const reconnectMaxDelay = options?.reconnectMaxDelay ?? DEFAULT_RECONNECT_MAX_DELAY;
  const initialConnectTimeout = options?.initialConnectTimeout ?? DEFAULT_INITIAL_CONNECT_TIMEOUT;

  let ws: WebSocket | null = null;
  let pingTimer: ReturnType<typeof setInterval> | null = null;
  let timeoutTimer: ReturnType<typeof setInterval> | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let initialTimer: ReturnType<typeof setTimeout> | null = null;
  let tokenPollTimer: ReturnType<typeof setInterval> | null = null;
  let reconnectAttempts = 0;
  let stopped = false;
  let shutdownReceived = false;
  let firstConnection = true;
  let notifiedDown = false;

  function connect(): void {
    if (stopped) return;
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
      return;
    }

    // Same-origin WS sends HttpOnly session cookie; dev bypass does not require a readable token.
    const canOpenWs = import.meta.env.DEV || isWsSameOriginAsPage() || !!readAccessTokenCookie();
    if (!canOpenWs) {
      startTokenPoll();
      return;
    }

    const url = buildHealthWsUrl();
    try {
      ws = new WebSocket(url);
    } catch {
      isAlive.value = false;
      scheduleReconnect();
      return;
    }

    if (firstConnection) {
      clearInitialTimer();
      initialTimer = setTimeout(() => {
        if (!connected.value && firstConnection) {
          handleServerDown();
        }
      }, initialConnectTimeout);
    }

    ws.onopen = () => {
      connected.value = true;
      isAlive.value = true;
      reconnectAttempts = 0;
      firstConnection = false;
      notifiedDown = false;
      clearInitialTimer();
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
          handleServerDown();
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
        if (!import.meta.env.DEV && !isWsSameOriginAsPage() && !readAccessTokenCookie()) {
          startTokenPoll();
        } else {
          scheduleReconnect();
        }
      }
    };

    ws.onerror = () => {
      // onclose will fire after onerror
    };
  }

  function disconnect(): void {
    stopped = true;
    clearInitialTimer();
    stopTokenPoll();
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

  function clearInitialTimer() {
    if (initialTimer) {
      clearTimeout(initialTimer);
      initialTimer = null;
    }
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
        handleServerDown();
        return;
      }
      const elapsed = Date.now() - lastPongAt.value;
      if (elapsed > pongTimeout) {
        handleServerDown();
      }
    }, 5_000);
  }

  function stopTimeoutCheck(): void {
    if (timeoutTimer) {
      clearInterval(timeoutTimer);
      timeoutTimer = null;
    }
  }

  function startTokenPoll(): void {
    stopTokenPoll();
    tokenPollTimer = setInterval(() => {
      if (readAccessTokenCookie()) {
        stopTokenPoll();
        connect();
      }
    }, 3_000);
  }

  function stopTokenPoll(): void {
    if (tokenPollTimer) {
      clearInterval(tokenPollTimer);
      tokenPollTimer = null;
    }
  }

  async function handleServerDown(): Promise<void> {
    isAlive.value = false;
    if (notifiedDown) return;
    notifiedDown = true;

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
