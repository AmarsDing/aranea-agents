import { ref } from 'vue';
import { i18n } from '../../i18n';
import { buildHealthWsUrl } from './api';
import { getCurrentAdmin } from '../admin/api';
import { useAuthStore } from '../../stores/auth';
import { Notify } from 'quasar';
import { getBackendOrigin, isWsSameOriginAsPage, isLocalHttpOrigin, readAccessTokenCookie } from '../../config/runtime';
import {
  HEARTBEAT_PING_INTERVAL_MS,
  HEARTBEAT_PONG_TIMEOUT_MS,
  HEARTBEAT_RECONNECT_BASE_DELAY_MS,
  HEARTBEAT_RECONNECT_MAX_DELAY_MS,
  HEARTBEAT_INITIAL_CONNECT_TIMEOUT_MS,
  HEARTBEAT_SHUTDOWN_RECOVERY_POLL_MS,
  HEARTBEAT_SHUTDOWN_RECOVERY_TIMEOUT_MS,
} from '../constants/timeouts';

export type ServerHeartbeatOptions = {
  pingInterval?: number;
  pongTimeout?: number;
  reconnectBaseDelay?: number;
  reconnectMaxDelay?: number;
  initialConnectTimeout?: number;
};

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
  const baseUrl = origin ? `${origin}/healthz` : '/healthz';
  try {
    const resp = await fetch(baseUrl, { method: 'GET', cache: 'no-store', signal: AbortSignal.timeout(5000) });
    return resp.ok;
  } catch {
    return false;
  }
}

/** DEV 专用：后端优雅关闭后轮询 /healthz 等待其完成重启，超时视为未恢复。 */
async function waitForBackendRecovery(): Promise<boolean> {
  const deadline = Date.now() + HEARTBEAT_SHUTDOWN_RECOVERY_TIMEOUT_MS;
  while (Date.now() < deadline) {
    if (await checkBackendHealth()) return true;
    await new Promise((resolve) => setTimeout(resolve, HEARTBEAT_SHUTDOWN_RECOVERY_POLL_MS));
  }
  return false;
}

async function shouldForceLogout(shutdown: boolean): Promise<boolean> {
  if (shutdown) {
    // DEV 下后端热重启是常态：等 /healthz 恢复后重新校验会话，仍有效则不强制登出；
    // 生产保持原语义——收到 server_shutdown 即要求重新登录。
    if (import.meta.env.DEV) {
      if (!(await waitForBackendRecovery())) return true;
      try {
        await getCurrentAdmin();
        return false;
      } catch {
        return true;
      }
    }
    return true;
  }
  if (import.meta.env.DEV) return false;
  if (!(await checkBackendHealth())) return true;
  const auth = useAuthStore();
  if (!auth.user) return false;
  try {
    await getCurrentAdmin();
    return false;
  } catch {
    return true;
  }
}

function createHeartbeat(options?: ServerHeartbeatOptions) {
  const pingInterval = options?.pingInterval ?? HEARTBEAT_PING_INTERVAL_MS;
  const pongTimeout = options?.pongTimeout ?? HEARTBEAT_PONG_TIMEOUT_MS;
  const reconnectBaseDelay = options?.reconnectBaseDelay ?? HEARTBEAT_RECONNECT_BASE_DELAY_MS;
  const reconnectMaxDelay = options?.reconnectMaxDelay ?? HEARTBEAT_RECONNECT_MAX_DELAY_MS;
  const initialConnectTimeout = options?.initialConnectTimeout ?? HEARTBEAT_INITIAL_CONNECT_TIMEOUT_MS;

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
  let degradedNotified = false;
  let dismissDownNotify: (() => void) | null = null;
  let pageHidden = false;

  if (typeof document !== 'undefined') {
    pageHidden = document.visibilityState === 'hidden';
    document.addEventListener('visibilitychange', () => {
      pageHidden = document.visibilityState === 'hidden';
      if (!pageHidden) {
        touchLastAlive();
        sendPingNow();
        if (!connected.value && !stopped) {
          scheduleReconnect();
        }
      }
    });
  }

  function touchLastAlive(): void {
    lastPongAt.value = Date.now();
    isAlive.value = true;
  }

  function sendPingNow(): void {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ direction: 'client_to_server', channel: 'system', type: 'ping' }));
    }
  }

  function connect(): void {
    if (stopped) return;
    if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
      return;
    }

    const canOpenWs = import.meta.env.DEV || isWsSameOriginAsPage() || !!readAccessTokenCookie() || isLocalHttpOrigin();
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
          void handleProbeFailure();
        }
      }, initialConnectTimeout);
    }

    ws.onopen = () => {
      connected.value = true;
      isAlive.value = true;
      reconnectAttempts = 0;
      firstConnection = false;
      notifiedDown = false;
      degradedNotified = false;
      shutdownReceived = false;
      clearInitialTimer();
      touchLastAlive();
      startPing();
      startTimeoutCheck();
      sendPingNow();
    };

    ws.onmessage = (ev: MessageEvent) => {
      try {
        const msg = JSON.parse(ev.data as string);
        if (msg.direction === 'server_to_client') {
          touchLastAlive();
        }
        if (msg.type === 'server_shutdown') {
          shutdownReceived = true;
          void handleProbeFailure();
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
      ws.close(1000, 'heartbeat disconnect');
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
    pingTimer = setInterval(sendPingNow, pingInterval);
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
      if (pageHidden) return;
      if (!lastPongAt.value) {
        void handleProbeFailure();
        return;
      }
      const elapsed = Date.now() - lastPongAt.value;
      if (elapsed > pongTimeout) {
        void handleProbeFailure();
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

  function clearDownNotification(): void {
    dismissDownNotify?.();
    dismissDownNotify = null;
    notifiedDown = false;
    shutdownReceived = false;
    degradedNotified = false;
    probeInFlight = false;
    isAlive.value = true;
    if (!stopped) {
      reconnectProbe();
    }
  }

  function reconnectProbe(): void {
    notifiedDown = false;
    if (ws) {
      ws.onclose = null;
      ws.close(1000, 'heartbeat reconnect');
      ws = null;
    }
    connected.value = false;
    scheduleReconnect();
  }

  let probeInFlight = false;

  async function handleProbeFailure(): Promise<void> {
    // 防重入：DEV 关机恢复最长等待 60s，期间 pong 超时检查可能再次触发
    if (probeInFlight) return;
    probeInFlight = true;
    try {
      isAlive.value = false;
      const wasShutdown = shutdownReceived;
      let dismissRecovering: (() => void) | null = null;
      if (wasShutdown && import.meta.env.DEV) {
        dismissRecovering = Notify.create({
          type: 'info',
          message: i18n.global.t('heartbeat.serverRestarting'),
          timeout: 0,
          group: 'heartbeat-recovering',
        }) as () => void;
      }
      const forceLogout = await shouldForceLogout(wasShutdown);
      dismissRecovering?.();

      if (!forceLogout) {
        if (wasShutdown) {
          // DEV：后端已恢复且会话仍然有效，无需重新登录
          shutdownReceived = false;
          Notify.create({ type: 'positive', message: i18n.global.t('heartbeat.serverRecovered'), timeout: 3000 });
        } else if (!degradedNotified) {
          degradedNotified = true;
          Notify.create({
            type: 'info',
            message: i18n.global.t('heartbeat.connectionDegraded'),
            timeout: 4000,
          });
        }
        reconnectProbe();
        return;
      }

      if (notifiedDown) return;
      notifiedDown = true;

      const auth = useAuthStore();
      auth.user = null;
      auth.sessionChecked = true;

      dismissDownNotify?.();
      dismissDownNotify = Notify.create({
        type: 'warning',
        message: shutdownReceived ? i18n.global.t('heartbeat.serverShutdown') : i18n.global.t('heartbeat.connectionTimeout'),
        timeout: 0,
        actions: [{ label: i18n.global.t('heartbeat.relogin'), color: 'white', handler: () => {} }],
      }) as () => void;

      const { default: router } = await import('../../router');
      const current = router.currentRoute.value;
      if (current.path !== '/login') {
        router.replace({ path: '/login', query: { redirect: current.fullPath } });
      }
    } finally {
      probeInFlight = false;
    }
  }

  connect();

  return {
    isAlive,
    lastPongAt,
    connected,
    connect,
    disconnect,
    clearDownNotification,
  };
}

/** Dismiss persistent "server down / re-login" banner and resume heartbeat after login. */
export function clearServerDownNotify(): void {
  heartbeatInstance?.clearDownNotification();
}
