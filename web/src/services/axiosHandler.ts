import axios, { type AxiosError } from "axios";
import { Notify } from "quasar";
import { getBackendOrigin } from "../config/runtime";

/**
 * **`kratosApi`**：`requestHandler` + `create*Service()` → **`/v1/...`**（含 **`memory/v1`**）；**`chat`**：`ChatServiceHTTPServer`（**`/v1/chat/*`**）；**`skills/import*`**：本进程 **`RegisterSkillImportHTTPServer`**（multipart + JSON）。
 *
 * {@link syncHttpClients} 在 **`loadRuntimeConfig`** 之后刷新 **`kratosApi`** 的 **`baseURL`**（见 **`getBackendOrigin()`**）。
 */
export const kratosApi = axios.create({
  baseURL: getBackendOrigin(),
  timeout: 15000,
  // Session cookie is host-scoped; call API with credentials so login works when CORS allows Origin (see CorsDevFilter).
  withCredentials: true
});

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
      Notify.create({
        type: "warning",
        message: kratosMsg ?? `请求错误 (${status})`,
        timeout: 3500
      });
    }

    return Promise.reject(err);
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

/** 与 proto 生成的 HTTP 客户端（`v1/...`）双参 handler 签名一致；`meta` 可供拦截器或日志使用 */
export function requestHandler(
  { path, method, body }: Request,
  _meta?: { service: string; method: string }
): Promise<unknown> {
  const headers: Record<string, string> = {};
  if (method === "POST" || method === "PUT" || method === "PATCH") {
    headers["Content-Type"] = "application/json";
  }
  return kratosApi.request({
      url: "/" + path.replace(/^\//, ""),
      method,
      data: body ?? undefined,
      headers
    })
    .then((res) => res.data);
}
