import type { AdkLiveEvent, AdkLiveRequest } from "./types";

export type AdkLiveWebSocketOptions = {
  onMessage: (data: string) => void;
  onError?: (e: Event) => void;
  onClose?: (ev: CloseEvent) => void;
  onOpen?: () => void;
};

/**
 * 与 adk-web 对齐：`wss?://{host}/run_live?app_name&user_id&session_id`。
 * 发送前将 `blob.data` 转为 **base64** 字符串，与 adk 前端一致。
 */
export class AdkLiveWebSocket {
  private socket: WebSocket | null = null;
  private readonly options: AdkLiveWebSocketOptions;

  constructor(options: AdkLiveWebSocketOptions) {
    this.options = options;
  }

  get connected(): boolean {
    return this.socket != null && this.socket.readyState === WebSocket.OPEN;
  }

  connect(url: string) {
    this.close();
    const ws = new WebSocket(url);
    this.socket = ws;
    ws.onopen = () => {
      this.options.onOpen?.();
    };
    ws.onmessage = (ev) => {
      if (typeof ev.data === "string") {
        this.options.onMessage(ev.data);
      }
    };
    ws.onerror = (e) => {
      this.options.onError?.(e);
    };
    ws.onclose = (ev) => {
      this.socket = null;
      this.options.onClose?.(ev);
    };
  }

  /**
   * 与 adk-web 一致：JSON 发送，`blob.data` 先转 base64。
   */
  sendMessage(payload: AdkLiveRequest) {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
      return;
    }
    const out: AdkLiveRequest = { ...payload };
    if (out.blob?.data) {
      const buffer =
        out.blob.data instanceof ArrayBuffer
          ? out.blob.data
          : out.blob.data.buffer.slice(
              out.blob.data.byteOffset,
              out.blob.data.byteOffset + out.blob.data.byteLength
            );
      (out as { blob: { mime_type: string; data: string } }).blob = {
        mime_type: out.blob.mime_type,
        data: arrayBufferToBase64(buffer)
      };
    }
    this.socket.send(JSON.stringify(out));
  }

  /**
   * 将服务端下行文本解析为事件（adk 里多为 JSON 字符串，音频在 inlineData）。
   */
  static parseEvent(jsonText: string): AdkLiveEvent | null {
    try {
      return JSON.parse(jsonText) as AdkLiveEvent;
    } catch {
      return null;
    }
  }

  close() {
    if (this.socket) {
      this.socket.onclose = null;
      this.socket.onerror = null;
      this.socket.onmessage = null;
      this.socket.onopen = null;
      try {
        this.socket.close();
      } catch {
        // ignore
      }
      this.socket = null;
    }
  }
}

function arrayBufferToBase64(buffer: ArrayBuffer): string {
  let binary = "";
  const bytes = new Uint8Array(buffer);
  for (let i = 0; i < bytes.byteLength; i++) {
    binary += String.fromCharCode(bytes[i]!);
  }
  return btoa(binary);
}
