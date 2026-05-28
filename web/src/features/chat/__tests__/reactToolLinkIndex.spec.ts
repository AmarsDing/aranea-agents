import { describe, expect, it } from "vitest";
import { buildReactToolLinkIndex } from "../reactToolLinkIndex";
import type { Message } from "../types";

describe("buildReactToolLinkIndex", () => {
  it("indexes linked tool ids once per session", () => {
    const messages: Message[] = [
      {
        id: "m1",
        session_id: "s1",
        parent_message_id: "",
        turn_id: "",
        turn_number: 1,
        seq_in_turn: 0,
        role: "assistant",
        content_markdown: "/*ACTION*/\nfunctions.search",
        model_name: "",
        token_in: 0,
        token_out: 0,
        latency_ms: 0,
        status: "ok",
        attachments_count: 0,
        options_json: "{}",
        error_message: "",
        created_at: "2026-05-21T10:00:00Z",
      },
      {
        id: "m2",
        session_id: "s1",
        parent_message_id: "",
        turn_id: "",
        turn_number: 1,
        seq_in_turn: 0,
        role: "assistant",
        content_markdown: "",
        model_name: "",
        token_in: 0,
        token_out: 0,
        latency_ms: 0,
        status: "tool_ok",
        attachments_count: 0,
        options_json: JSON.stringify({
          schema: "chat.activity/v1",
          tool_event: {
            id: "tc-1",
            phase: "after",
            status: "success",
            agent_id: "a1",
            agent_key: "agent",
            agent_name: "Agent",
            agent_icon: "",
            tool_name: "search",
            tool_label: "search",
            occurred_at: "2026-05-21T10:00:01Z",
          },
        }),
        error_message: "",
        created_at: "2026-05-21T10:00:01Z",
      },
    ];
    const idx = buildReactToolLinkIndex(messages);
    expect(idx.linkedToolIds.has("tc-1")).toBe(true);
    expect(idx.stepsByAssistantIndex.get(0)?.[0]?.linkedTools).toHaveLength(1);
  });
});
