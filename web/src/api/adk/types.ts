/**
 * 与 adk-web / Google ADK 流式、实时接口对齐的客户端类型子集，便于在应用内统一引用。
 * @see https://github.com/google/adk
 */

export type AdkRole = "user" | "model" | "system" | string;

export type AdkFunctionResponse = {
  id?: string;
  name: string;
  response: Record<string, unknown>;
};

export type AdkContentPart = {
  text?: string;
  functionResponse?: AdkFunctionResponse;
  inlineData?: { mimeType?: string; data: string };
  [k: string]: unknown;
};

export type AdkGenAiContent = {
  role: AdkRole;
  parts: AdkContentPart[];
};

/**
 * 对应 adk-web `POST /run_sse` 的 JSON 请求体
 */
export type AdkRunSseRequest = {
  appName: string;
  userId: string;
  sessionId: string | undefined;
  newMessage: {
    role: AdkRole;
    parts: AdkContentPart[];
  };
  functionCallEventId?: string;
  streaming?: boolean;
  stateDelta?: unknown;
  invocationId?: string;
};

export type AdkLlmResponse = {
  content?: AdkGenAiContent;
  error?: string;
  errorMessage?: string;
  errorCode?: string;
  longRunningToolIds?: string[];
  actions?: unknown;
};

/**
 * 下行 WebSocket 文本（JSON 字符串）解析为事件时，在 adk-web 中与 LlmResponse 结构兼容。
 */
export type AdkLiveEvent = AdkLlmResponse & {
  id?: string;
  author?: string;
  content?: AdkGenAiContent;
};

/**
 * 上行 `run_live`：音频/视频分片
 */
export type AdkLiveBlob = {
  mime_type: string;
  data: ArrayBuffer | ArrayBufferView;
};

export type AdkLiveRequest = {
  content?: unknown;
  blob?: AdkLiveBlob;
  close?: boolean;
  model_config?: unknown;
};
