import { describe, expect, it, vi } from "vitest";
import { EnvelopeDispatcher } from "../dispatcher";
import type { Envelope } from "../envelope";
import { bindStreamHandlers, patchStreamingEnvelope } from "../streamHandlers";
import type { Message } from "../types";

function env(partial: Partial<Envelope> & { type: Envelope["type"] }): Envelope {
  return {
    id: "e1",
    author: "test",
    session_id: "sess-1",
    timestamp: "",
    version: 1,
    ...partial,
  };
}

describe("bindStreamHandlers", () => {
  it("patches text_delta then text_done and completes with reload", async () => {
    const rows: Record<string, Message[]> = { "sess-1": [] };
    const markSendingDone = vi.fn();
    const onReloadAfterCompletion = vi.fn().mockResolvedValue(undefined);
    const dispatcher = new EnvelopeDispatcher();

    const stream = {
      onType: (type: string | string[], handler: (e: Envelope) => void) =>
        dispatcher.onType(type as Envelope["type"] | Envelope["type"][], handler),
    };

    bindStreamHandlers(stream as any, {
      sessionId: "sess-1",
      resolveActiveSessionId: () => "sess-1",
      getMessages: (sid) => rows[sid] ?? [],
      setMessages: (sid, next) => {
        rows[sid] = next;
      },
      markSendingDone,
      onRunStatus: vi.fn(),
      onErrorNotify: vi.fn(),
      onReloadAfterCompletion,
      setLastIntentPass: vi.fn(),
    });

    dispatcher.dispatch(
      env({
        type: "text_delta",
        content: { text: "Hello", reasoning: "" },
      })
    );
    expect(rows["sess-1"]!.some((m) => m.id === "ws-stream-sess-1")).toBe(true);
    expect(rows["sess-1"]!.find((m) => m.id === "ws-stream-sess-1")?.content_markdown).toBe("Hello");

    dispatcher.dispatch(
      env({
        type: "text_done",
        content: { text: "Hello world", reasoning: "" },
      })
    );
    expect(rows["sess-1"]!.find((m) => m.id === "ws-stream-sess-1")?.status).toBe("ok");

    dispatcher.dispatch(env({ type: "runner_completion" }));
    expect(markSendingDone).toHaveBeenCalled();
    expect(onReloadAfterCompletion).toHaveBeenCalledWith("sess-1");
  });

  it("ignores deltas for inactive session", () => {
    const rows: Record<string, Message[]> = { "sess-1": [] };
    const dispatcher = new EnvelopeDispatcher();
    const stream = {
      onType: (type: string | string[], handler: (e: Envelope) => void) =>
        dispatcher.onType(type as Envelope["type"] | Envelope["type"][], handler),
    };

    bindStreamHandlers(stream as any, {
      sessionId: "sess-1",
      resolveActiveSessionId: () => "sess-2",
      getMessages: (sid) => rows[sid] ?? [],
      setMessages: (sid, next) => {
        rows[sid] = next;
      },
      markSendingDone: vi.fn(),
      onRunStatus: vi.fn(),
      onErrorNotify: vi.fn(),
      onReloadAfterCompletion: vi.fn(),
      setLastIntentPass: vi.fn(),
    });

    dispatcher.dispatch(
      env({
        type: "text_delta",
        content: { text: "skip", reasoning: "" },
      })
    );
    expect(rows["sess-1"]).toEqual([]);
  });

  it("patchStreamingEnvelope creates and updates streaming row", () => {
    const next = patchStreamingEnvelope([], "sess-1", "ws-stream-sess-1", {
      id: "e1",
      type: "text_delta",
      author: "test",
      session_id: "sess-1",
      timestamp: "",
      version: 1,
      content: { text: "Hi", reasoning: "" },
    }, false);
    expect(next).toHaveLength(1);
    expect(next[0]?.content_markdown).toBe("Hi");
    expect(next[0]?.status).toBe("streaming");
  });
});
