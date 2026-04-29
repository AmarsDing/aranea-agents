type RuntimeConfig = {
  /**
   * 后端 **Origin**（协议 + 主机 [:端口]），**不带路径**。
   * 与 Kratos Admin（`/v1/...`）、遗留 REST（`/api/v1/...`）共用同一网关来源，不在此做「两套根」分叉。
   */
  backendUrl?: string;
  /**
   * ADK HTTP 流式接口所在 **Origin**（无路径），如 `http://127.0.0.1:8000`。
   * 用于 `POST {origin}/run_sse`，与 `/api/v1` 可指向不同服务。
   */
  adkStreamOrigin?: string;
  /**
   * WebSocket 主机，格式 `host:port`，如 `127.0.0.1:8000`。
   * 用于 `ws(s)://{host}/run_live?...`
   */
  adkWsHost?: string;
};

let runtimeConfig: RuntimeConfig = {};

export async function loadRuntimeConfig(): Promise<void> {
  try {
    const resp = await fetch("/assets/config/runtime-config.json", { cache: "no-store" });
    if (!resp.ok) return;
    runtimeConfig = await resp.json();
  } catch {
    runtimeConfig = {};
  }
}

/**
 * 网关 **Origin**，与 Kratos Admin HTTP（`v1/...` 相对其根）一致。
 * 新能力请通过 **proto + `web/src/services`** 以 Kratos 方式接入。
 */
export function getBackendOrigin(): string {
  if (runtimeConfig.backendUrl && runtimeConfig.backendUrl.trim().length > 0) {
    return runtimeConfig.backendUrl.replace(/\/$/, "");
  }
  if (import.meta.env.DEV) {
    return "";
  }
  return "http://localhost:8080";
}

/** @deprecated 使用 {@link getBackendOrigin} */
export const getAdminServiceBaseURL = getBackendOrigin;

/**
 * 遗留手写 REST（如 `web/src/api/client.ts`）的 axios 前缀：**同一 `getBackendOrigin()` + 路径段 `/api/v1`**。
 * 新接口优先在 proto 中定义并由 `make api` 生成，不再增加此类路径族。
 */
export function getBackendBaseURL(): string {
  const origin = getBackendOrigin();
  if (origin === "") {
    return "/api/v1";
  }
  return `${origin}/api/v1`;
}

/**
 * tx7do SSE 监听独立端口（server.sse）；开发环境由 devServer 将 `/sse` 代理到该端口，前端用同源 `/sse/...` 避免跨域。
 */
export function getSseBaseURL(): string {
  const origin = getBackendOrigin();
  if (origin === "") {
    return "/sse";
  }
  return `${origin}/sse`;
}

/**
 * 用于 `POST .../run_sse` 的绝对 URL。
 * 未配置 `adkStreamOrigin` 时：开发环境走同源相对路径 `/run_sse`（由 devServer 代理到后端/ADK）；生产为当前站点 `origin`。
 */
export function getAdkRunSseUrl(): string {
  if (runtimeConfig.adkStreamOrigin && runtimeConfig.adkStreamOrigin.trim() !== "") {
    return `${runtimeConfig.adkStreamOrigin.replace(/\/$/, "")}/run_sse`;
  }
  if (import.meta.env.DEV) {
    return "/run_sse";
  }
  if (typeof window !== "undefined" && window.location?.origin) {
    return `${window.location.origin}/run_sse`;
  }
  return "http://localhost:8080/run_sse";
}

/**
 * 与 ADK 对齐：`ws(s)://{host}/run_live?app_name&user_id&session_id`
 */
export function buildAdkLiveUrl(params: {
  appName: string;
  userId: string;
  sessionId: string;
}): string {
  const protocol = typeof window !== "undefined" && window.location?.protocol === "https:" ? "wss" : "ws";
  const host = resolveAdkWsHost();
  const q = new URLSearchParams({
    app_name: params.appName,
    user_id: params.userId,
    session_id: params.sessionId
  });
  return `${protocol}://${host}/run_live?${q.toString()}`;
}

function resolveAdkWsHost(): string {
  if (runtimeConfig.adkWsHost && runtimeConfig.adkWsHost.trim() !== "") {
    return runtimeConfig.adkWsHost.trim();
  }
  if (import.meta.env.DEV) {
    return "127.0.0.1:8000";
  }
  if (typeof window !== "undefined" && window.location?.host) {
    return window.location.host;
  }
  return "127.0.0.1:8080";
}
