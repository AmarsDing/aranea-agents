import axios from "axios";
import { getBackendBaseURL, getBackendOrigin } from "../config/runtime";

/** Kratos / proto HTTP（相对网关 Origin，路径如 `v1/...`） */
export const kratosApi = axios.create({
  baseURL: getBackendOrigin(),
  timeout: 15000
});

export const legacyRestApi = axios.create({
  baseURL: getBackendBaseURL(),
  timeout: 15000
});

/**
 * 配置更新后刷新两端 baseURL（与 `loadRuntimeConfig` 之后在 boot 中调用一次）。
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
