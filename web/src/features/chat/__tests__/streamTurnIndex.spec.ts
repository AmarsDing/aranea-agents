import { describe, expect, it } from "vitest";
import { groupMessagesByTurn } from "../groupMessagesByTurn";
import { patchStreamingEnvelope } from "../streamHandlers";
import { upsertToolMessage } from "../envelopeToolCall";
import {
  inferAssistantStreamTurnIndex,
  inferToolActivityTurnIndex,
  realignEphemeralTurnIndexes,
} from "../streamTurnIndex";
import type { Envelope, Message } from "../types";

function msg(partial: Partial<Message> & Pick<Message, "id" | "role">): Message {
  return {
    session_id: "sess-1",
    parent_message_id: "",
    turn_index: partial.turn_index ?? 1,
    content_markdown: partial.content_markdown ?? "",
    model_name: "",
    token_in: 0,
    token_out: 0,
    latency_ms: 0,
    status: partial.status ?? "ok",
    attachments_count: 0,
    options_json: partial.options_json ?? "",
    error_message: "",
    created_at: partial.created_at ?? "2026-05-23T00:00:00Z",
    ...partial,
  };
}

describe("streamTurnIndex", () => {
  it("infers assistant and tool turn_index from latest user turn", () => {
    const history = [msg({ id: "u1", role: "user", turn_index: 5, content_markdown: "feishu" })];
    expect(inferAssistantStreamTurnIndex(history)).toBe(6);
    expect(inferToolActivityTurnIndex(history)).toBe(5);
  });

  it("uses max user turn_index regardless of message order", () => {
    const history = [
      msg({ id: "u-old", role: "user", turn_index: 3, content_markdown: "old" }),
      msg({ id: "ws-stream-sess-1", role: "assistant", turn_index: 1, status: "streaming" }),
      msg({ id: "u-new", role: "user", turn_index: 7, content_markdown: "feishu" }),
    ];
    expect(inferAssistantStreamTurnIndex(history)).toBe(8);
    const aligned = realignEphemeralTurnIndexes(history);
    expect(aligned.find((m) => m.id === "ws-stream-sess-1")?.turn_index).toBe(8);
  });

  it("realigns orphaned ws-stream and tool rows onto active turn", () => {
    const misaligned = [
      msg({ id: "u1", role: "user", turn_index: 5, content_markdown: "feishu" }),
      msg({ id: "ws-stream-sess-1", role: "assistant", turn_index: 1, status: "streaming" }),
      msg({
        id: "act-t1",
        role: "assistant",
        turn_index: 0,
        status: "tool_running",
        options_json: '{"schema":"chat.activity/v1","tool_event":{"id":"t1","tool_name":"search"}}',
      }),
    ];
    const aligned = realignEphemeralTurnIndexes(misaligned);
    expect(aligned.find((m) => m.id === "ws-stream-sess-1")?.turn_index).toBe(6);
    expect(aligned.find((m) => m.id === "act-t1")?.turn_index).toBe(5);
  });

  it("groups realigned channel stream into the Feishu user turn block", () => {
    const env: Envelope = {
      id: "e1",
      type: "text_delta",
      author: "agent",
      session_id: "sess-1",
      timestamp: "",
      version: 1,
      content: { text: "answer", reasoning: "thinking…", is_partial: true },
    };
    const withStream = patchStreamingEnvelope(
      [msg({ id: "u1", role: "user", turn_index: 5, content_markdown: "feishu" })],
      "sess-1",
      "ws-stream-sess-1",
      env,
      false
    );
    const withTool = upsertToolMessage(withStream, "sess-1", {
      id: "e2",
      type: "tool_call",
      author: "agent",
      session_id: "sess-1",
      timestamp: "",
      version: 1,
      tool_call: {
        id: "tc-1",
        name: "search",
        arguments_json: "{}",
        status: "running",
      },
    }, "before");

    const blocks = groupMessagesByTurn(withTool);
    expect(blocks).toHaveLength(1);
    expect(blocks[0]?.user?.id).toBe("u1");
    expect(blocks[0]?.tools).toHaveLength(1);
    expect(blocks[0]?.assistant?.id).toBe("ws-stream-sess-1");
  });
});
