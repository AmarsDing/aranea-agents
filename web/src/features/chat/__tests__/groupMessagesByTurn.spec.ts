import { describe, expect, it } from "vitest";
import { groupMessagesByTurn, lastAssistantTurnBlockIndex } from "../groupMessagesByTurn";
import type { Message } from "../types";

function msg(partial: Partial<Message> & Pick<Message, "id" | "role" | "turn_index">): Message {
  return {
    session_id: "s1",
    parent_message_id: "",
    content_markdown: partial.content_markdown ?? "body",
    model_name: "",
    token_in: 0,
    token_out: 0,
    latency_ms: partial.latency_ms ?? 0,
    status: partial.status ?? "ok",
    attachments_count: 0,
    options_json: partial.options_json ?? "",
    error_message: "",
    created_at: partial.created_at ?? "2026-05-23T00:00:00Z",
    ...partial,
  };
}

describe("groupMessagesByTurn", () => {
  it("groups user, tools, assistant into one turn block", () => {
    const messages: Message[] = [
      msg({ id: "u1", role: "user", turn_index: 1, content_markdown: "hi" }),
      msg({
        id: "t1",
        role: "assistant",
        turn_index: 1,
        content_markdown: "",
        options_json: '{"schema":"chat.activity/v1","tool_event":{}}',
        status: "tool_ok",
        latency_ms: 1200,
      }),
      msg({ id: "a1", role: "assistant", turn_index: 2, content_markdown: "done" }),
    ];
    const blocks = groupMessagesByTurn(messages);
    expect(blocks).toHaveLength(1);
    expect(blocks[0]?.user?.id).toBe("u1");
    expect(blocks[0]?.tools).toHaveLength(1);
    expect(blocks[0]?.assistant?.id).toBe("a1");
  });

  it("splits multiple turns by turn_index", () => {
    const messages: Message[] = [
      msg({ id: "u1", role: "user", turn_index: 1 }),
      msg({ id: "a1", role: "assistant", turn_index: 2 }),
      msg({ id: "u2", role: "user", turn_index: 3 }),
      msg({ id: "a2", role: "assistant", turn_index: 4 }),
    ];
    expect(groupMessagesByTurn(messages)).toHaveLength(2);
  });

  it("merges orphan tool-only blocks into previous user turn", () => {
    const messages: Message[] = [
      msg({ id: "u1", role: "user", turn_index: 1 }),
      msg({
        id: "t1",
        role: "assistant",
        turn_index: 3,
        content_markdown: "",
        options_json: '{"schema":"chat.activity/v1"}',
        status: "tool_ok",
      }),
      msg({
        id: "t2",
        role: "assistant",
        turn_index: 5,
        content_markdown: "",
        options_json: '{"schema":"chat.activity/v1"}',
        status: "tool_ok",
      }),
    ];
    const blocks = groupMessagesByTurn(messages);
    expect(blocks).toHaveLength(1);
    expect(blocks[0]?.tools).toHaveLength(2);
  });

  it("lastAssistantTurnBlockIndex anchors on assistant body", () => {
    const blocks = groupMessagesByTurn([
      msg({ id: "u1", role: "user", turn_index: 1 }),
      msg({
        id: "t1",
        role: "assistant",
        turn_index: 1,
        options_json: '{"schema":"chat.activity/v1"}',
        content_markdown: "",
      }),
      msg({ id: "a1", role: "assistant", turn_index: 2, content_markdown: "answer" }),
    ]);
    expect(lastAssistantTurnBlockIndex(blocks)).toBe(0);
  });
});
