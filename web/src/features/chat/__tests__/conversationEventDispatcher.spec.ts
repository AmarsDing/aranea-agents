import { describe, expect, it } from "vitest";
import type { Envelope } from "../envelope";
import {
  conversationSourceFromEnvelope,
  projectConversationEnvelope,
} from "../conversationEventDispatcher";

function env(partial: Partial<Envelope>): Envelope {
  return {
    id: "e1",
    type: "run_status",
    author: "test",
    session_id: "sess-1",
    timestamp: "2026-05-27T00:00:00Z",
    version: 1,
    ...partial,
  };
}

describe("conversationEventDispatcher", () => {
  it("projects current-session channel completion into a hydrating turn event", () => {
    const projected = projectConversationEnvelope(
      env({
        source: "channel",
        turn_id: "turn-1",
        session_revision: 7,
        metadata: {
          status: "completed",
          delivery_status: "sent",
          channel_id: "ch-1",
          platform: "slack",
          peer_id: "peer-1",
        },
      }),
      { currentSessionId: "sess-1" }
    );

    expect(projected).toMatchObject({
      scope: "current-session",
      sessionId: "sess-1",
      turnId: "turn-1",
      source: "channel",
      revision: 7,
      status: "completed",
      hydrate: true,
      stream: false,
      delivery: {
        kind: "channel",
        channelId: "ch-1",
        platform: "slack",
        recipientId: "peer-1",
        status: "delivered",
      },
    });
  });

  it("routes non-current session events to inbox", () => {
    const projected = projectConversationEnvelope(env({ session_id: "sess-2" }), {
      currentSessionId: "sess-1",
    });
    expect(projected?.scope).toBe("inbox");
  });

  it("normalizes background source aliases", () => {
    expect(conversationSourceFromEnvelope(env({ source: "job" }))).toBe("durable");
    expect(conversationSourceFromEnvelope(env({ metadata: { source: "background" } }))).toBe("durable");
  });
});
