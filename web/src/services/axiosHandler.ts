import axios, { type AxiosError } from "axios";
import { Notify } from "quasar";
import { getBackendOrigin } from "../config/runtime";

/** Default HTTP timeout for admin CRUD APIs (ms). */
export const KRATOS_API_DEFAULT_TIMEOUT_MS = 30_000;
/** Chat / session sync can wait for long agent turns (align with channel first_byte_timeout_sec). */
export const KRATOS_API_LONG_TIMEOUT_MS = 120_000;

/**
 * **`kratosApi`**：`requestHandler` + `create*Service()` → **`/v1/...`**（含 **`memory/v1`**）；**`chat`**：`ChatServiceHTTPServer`（**`/v1/chat/*`**）；**`skills/import*`**：`SkillService`（proto HTTP + multipart ZIP on **`RegisterSkillImportMultipart`**）。
 *
 * {@link syncHttpClients} 在 **`loadRuntimeConfig`** 之后刷新 **`kratosApi`** 的 **`baseURL`**（见 **`getBackendOrigin()`**）。
 */
export const kratosApi = axios.create({
  baseURL: getBackendOrigin(),
  timeout: KRATOS_API_DEFAULT_TIMEOUT_MS,
  // Session cookie is host-scoped; call API with credentials so login works when CORS allows Origin (see CorsDevFilter).
  withCredentials: true
});

function resolveRequestTimeoutMs(path: string, override?: number): number {
  if (override != null && override > 0) return override;
  const p = path.replace(/^\//, "");
  if (p.startsWith("v1/chat/") || p.startsWith("v1/sessions")) {
    return KRATOS_API_LONG_TIMEOUT_MS;
  }
  if (p.startsWith("v1/model-catalog/sync")) {
    return KRATOS_API_LONG_TIMEOUT_MS;
  }
  return KRATOS_API_DEFAULT_TIMEOUT_MS;
}

function humanizeAxiosError(err: AxiosError): AxiosError {
  if (err.code === "ECONNABORTED" && !err.response) {
    err.message = "请求超时，请确认后端 admin 是否在 :8000 运行并重试";
  } else if (!err.response && (err.code === "ERR_NETWORK" || err.message === "Network Error")) {
    err.message = "无法连接后端，请确认 admin 是否在 :8000 运行";
  }
  return err;
}

// --- Response interceptor: map Kratos error envelopes to user-visible notifications ---

kratosApi.interceptors.response.use(
  (res) => res,
  async (err: AxiosError) => {
    const status = err.response?.status;

    // 401 → redirect to login
    if (status === 401) {
      const currentPath = window.location.pathname;
      if (!currentPath.startsWith("/login")) {
        window.location.href = `/login?redirect=${encodeURIComponent(currentPath)}`;
      }
      return Promise.reject(err);
    }

    // 429 → back-off notification (do not auto-retry here; caller handles retry logic)
    if (status === 429) {
      const retryAfter = err.response?.headers?.["retry-after"];
      const hint = retryAfter ? ` (retry after ${retryAfter}s)` : "";
      Notify.create({ type: "warning", message: `请求过于频繁，请稍后再试${hint}`, timeout: 4000 });
      return Promise.reject(err);
    }

    // Extract Kratos error envelope: { code, message, metadata }
    const data = err.response?.data as Record<string, unknown> | undefined;
    const kratosMsg = typeof data?.message === "string" ? data.message : undefined;

    if (status && status >= 500) {
      Notify.create({
        type: "negative",
        message: kratosMsg ?? `服务器错误 (${status})`,
        timeout: 5000,
        actions: [{ label: "关闭", color: "white" }]
      });
    } else if (status && status >= 400 && status !== 401 && status !== 404) {
      const skipNotify = Boolean(err.config?.skipErrorNotify);
      if (!skipNotify) {
        Notify.create({
          type: "warning",
          message: kratosMsg ?? `请求错误 (${status})`,
          timeout: 3500
        });
      }
    }

    return Promise.reject(humanizeAxiosError(err));
  }
);

/**
 * `loadRuntimeConfig` 之后在 boot 中调用一次：刷新 **`kratosApi`** 的 **`baseURL`**。
 */
export function syncHttpClients() {
  kratosApi.defaults.baseURL = getBackendOrigin();
}

/** @deprecated 使用 {@link syncHttpClients} */
export function syncXApiBaseURL() {
  syncHttpClients();
}

type Request = {
  path: string;
  method: string;
  body: string | null;
};

export type RequestMeta = {
  service: string;
  method: string;
  /** Suppress global 4xx toast; used for CreateAgent inline field errors. */
  skipErrorNotify?: boolean;
  /** Per-request timeout override (ms). */
  timeoutMs?: number;
};

/** 与 proto 生成的 HTTP 客户端（`v1/...`）双参 handler 签名一致；`meta` 可供拦截器或日志使用 */
export function requestHandler(
  { path, method, body }: Request,
  meta?: RequestMeta
): Promise<unknown> {
  const headers: Record<string, string> = {};
  if (method === "POST" || method === "PUT" || method === "PATCH") {
    headers["Content-Type"] = "application/json";
  }
  const skipErrorNotify = meta?.skipErrorNotify ?? meta?.method === "CreateAgent";
  const urlPath = "/" + path.replace(/^\//, "");
  return kratosApi
    .request({
      url: urlPath,
      method,
      data: body ?? undefined,
      headers,
      skipErrorNotify,
      timeout: resolveRequestTimeoutMs(path, meta?.timeoutMs)
    })
    .then((res) => res.data);
}
