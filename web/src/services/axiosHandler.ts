import axios from "axios";
import { getBackendOrigin } from "../config/runtime";

/**
 * **`kratosApi`**：`requestHandler` + `create*Service()` → **`/v1/...`**（含 **`memory/v1`**）；**`chat`**：`RegisterLegacyChatForwardHTTPServer`（**`LEGACY_REST_ORIGIN`** → **`/api/v1/chat/*`**）；**`skills/import*`**：本进程 **`RegisterSkillImportHTTPServer`**（multipart + JSON，不经遗留网关）。
 *
 * {@link syncHttpClients} 在 **`loadRuntimeConfig`** 之后刷新 **`kratosApi`** 的 **`baseURL`**（见 **`getBackendOrigin()`**）。
 */
export const kratosApi = axios.create({
  baseURL: getBackendOrigin(),
  timeout: 15000,
  // Session cookie is host-scoped; call API with credentials so login works when CORS allows Origin (see CorsDevFilter).
  withCredentials: true
});

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
