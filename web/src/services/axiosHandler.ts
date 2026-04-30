import axios from "axios";
import { getBackendBaseURL, getBackendOrigin } from "../config/runtime";

/**
 * **双 axios 实例**（同源网关下两套路径前缀；均由 {@link syncHttpClients} 在运行时刷新 baseURL）：
 *
 * - **`kratosApi`**：`requestHandler` + `create*Service()` → **`/v1/...`**（含 **`memory/v1`** 由 **`cmd/admin`** 处理）；**`chat`** / **`skills/import`** 等在配置了 **`LEGACY_REST_ORIGIN`** 时由 **`LegacyRESTProxyFilter`** 转发至 **`/api/v1/...`**（上游可为过渡期 shim，`pkg/backend` 已废弃）。
 * - **`legacyRestApi`**：仅其余仍直连 **`/api/v1/...`** 的手写 REST（若有）；收口进 Kratos 后删调用侧。
 *
 * 二者不是「两套业务接口」，而是同一网关下的 **两条 HTTP 前缀**；第 20 行仅在 **`syncHttpClients`** 中与 **`kratosApi`** 同步更新前缀。
 */
export const kratosApi = axios.create({
  baseURL: getBackendOrigin(),
  timeout: 15000
});

export const legacyRestApi = axios.create({
  baseURL: getBackendBaseURL(),
  timeout: 15000
});

/**
 * `loadRuntimeConfig` 之后在 boot 中调用一次：同时为 **`kratosApi`** 与 **`legacyRestApi`** 写入当前网关 Origin / **`/api/v1`** 前缀。
 */
export function syncHttpClients() {
  kratosApi.defaults.baseURL = getBackendOrigin();
  legacyRestApi.defaults.baseURL = getBackendBaseURL();
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
