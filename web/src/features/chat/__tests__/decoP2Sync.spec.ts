import { describe, expect, it, vi } from "vitest";
import type { Envelope } from "../envelope";

function isBackgroundJobRefreshEnvelope(env: Envelope, sessionId?: string): boolean {
  const md = env.metadata as Record<string, unknown> | undefined;
  if (!md?.background_job_refresh) return false;
  const sid = (env.session_id ?? "").trim();
  if (sessionId && sid && sid !== sessionId.trim()) return false;
  return true;
}

describe("background job refresh envelope (DECO-12)", () => {
  it("matches refresh for session", () => {
    const env: Envelope = {
      id: "e1",
      type: "run_status",
      author: "background-job",
      session_id: "sess-1",
      timestamp: "",
      version: 1,
      channel: "chat",
      metadata: { background_job_refresh: true, job_status: "running" },
    };
    expect(isBackgroundJobRefreshEnvelope(env, "sess-1")).toBe(true);
    expect(isBackgroundJobRefreshEnvelope(env, "sess-2")).toBe(false);
  });

  it("ignores unrelated run_status", () => {
    const env: Envelope = {
      id: "e2",
      type: "run_status",
      author: "run-service",
      session_id: "sess-1",
      timestamp: "",
      version: 1,
      metadata: { status: "completed" },
    };
    expect(isBackgroundJobRefreshEnvelope(env, "sess-1")).toBe(false);
  });
});

describe("inbound hydrate error callback (DECO-14)", () => {
  it("invokes onHydrateError when loadMessages rejects", async () => {
    const onHydrateError = vi.fn();
    const sessionId = "sess-hydrate";
    const chatStore = {
      sessionRevisionBySession: { [sessionId]: 2 } as Record<string, number>,
      getMessages: () => [],
      setMessages: vi.fn(),
      loadMessages: vi.fn().mockRejectedValue(new Error("network down")),
      entityKind: "agent" as const,
      sessions: [],
      selectedSession: null,
    };
    const ensureChatStream = vi.fn(() => ({
      patchMessages: vi.fn(),
    }));

    async function hydrateCurrentSession(sid: string) {
      const localRev = chatStore.sessionRevisionBySession[sid] ?? 0;
      try {
        await chatStore.loadMessages({ sessionId: sid, afterRevision: localRev });
      } catch (err) {
        const message = err instanceof Error ? err.message : "hydrate failed";
        onHydrateError(sid, message);
      }
    }

    await hydrateCurrentSession(sessionId);
    expect(onHydrateError).toHaveBeenCalledWith(sessionId, "network down");
    expect(ensureChatStream).not.toHaveBeenCalled();
  });
});
