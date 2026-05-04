import { getAdkRunSseUrl } from "../../config/runtime";
import type { AdkLlmResponse, AdkRunSseRequest } from "./types";

export type AdkRunSseHandlers = {
  onEvent: (event: AdkLlmResponse) => void;
  onError?: (error: unknown) => void;
  onComplete?: () => void;
};

export type AdkRunSseController = {
  /** 结束读取并中止 fetch */
  cancel: () => void;
  /** 流结束、出错或取消后 resolve */
  finished: Promise<void>;
};

/**
 * `POST /run_sse`，`Accept: text/event-stream`。
 * 与 adk-web 相同：fetch + ReadableStream，按行解析 `data:` 后 JSON，逐条回调。
 */
export function runSse(
  requestBody: AdkRunSseRequest,
  handlers: AdkRunSseHandlers,
  options?: { signal?: AbortSignal }
): AdkRunSseController {
  const ac = new AbortController();
  const signal = options?.signal;
  if (signal) {
    if (signal.aborted) {
      ac.abort();
    } else {
      signal.addEventListener("abort", () => ac.abort(), { once: true });
    }
  }

  const url = getAdkRunSseUrl();
  let lineBuffer = "";

  const finished = (async () => {
    try {
      const res = await fetch(url, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Accept: "text/event-stream"
        },
        body: JSON.stringify(requestBody),
        signal: ac.signal
      });

      if (!res.ok) {
        throw new Error(`run_sse: HTTP ${res.status}`);
      }
      if (!res.body) {
        throw new Error("run_sse: no response body");
      }

      const reader = res.body.getReader();
      const decoder = new TextDecoder("utf-8");
      for (;;) {
        const { value, done } = await reader.read();
        if (done) {
          lineBuffer = processSseBuffer(lineBuffer, handlers, true);
          return;
        }
        lineBuffer += decoder.decode(value, { stream: true });
        lineBuffer = processSseBuffer(lineBuffer, handlers, false);
      }
    } catch (e) {
      if (ac.signal.aborted) {
        return;
      }
      handlers.onError?.(e);
    } finally {
      handlers.onComplete?.();
    }
  })();

  return {
    cancel: () => {
      ac.abort();
    },
    finished
  };
}

function processSseBuffer(
  buffer: string,
  handlers: AdkRunSseHandlers,
  eof: boolean
): string {
  const lines = buffer.split(/\r?\n/);
  const toProcess = eof ? lines : lines.slice(0, -1);
  const rest = eof ? "" : lines[lines.length - 1] ?? "";

  for (const line of toProcess) {
    if (!line.startsWith("data:")) {
      continue;
    }
    const data = line.replace(/^data:\s*/, "").trim();
    if (!data) {
      continue;
    }
    try {
      handlers.onEvent(JSON.parse(data) as AdkLlmResponse);
    } catch {
      // 单行内仍非完整 JSON 时忽略（下一 chunk 会进入 buffer）
    }
  }

  if (eof && rest.trim().length) {
    const t = rest.trim();
    if (t.startsWith("data:")) {
      const data = t.replace(/^data:\s*/, "").trim();
      if (data) {
        try {
          handlers.onEvent(JSON.parse(data) as AdkLlmResponse);
        } catch {
          // ignore
        }
      }
    }
  }

  return rest;
}
