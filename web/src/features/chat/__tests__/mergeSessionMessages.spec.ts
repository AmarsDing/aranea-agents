import { describe, expect, it } from "vitest";
import type { Message } from "../types";
import { isActivityMessage, mergeSessionMessages } from "../mergeSessionMessages";

function msg(id: string, status = "ok", created = "2026-05-20T10:00:00Z"): Message {
  return {
    id,
    session_id: "sess-1",
    parent_message_id: "",
    turn_index: 0,
    role: "assistant",
    content_markdown: "",
    model_name: "",
    token_in: 0,
    token_out: 0,
    latency_ms: 0,
    status,
    attachments_count: 0,
    options_json: "",
    error_message: "",
    created_at: created,
  };
}

describe("mergeSessionMessages", () => {
  it("keeps streaming row while merging server history", () => {
    const server = [msg("u-1", "ok", "2026-05-20T10:00:00Z")];
    const local = [
      ...server,
      msg("ws-stream-sess-1", "streaming", "2026-05-20T10:00:01Z"),
      msg("act-tc-1", "tool_running", "2026-05-20T10:00:02Z"),
    ];
    const merged = mergeSessionMessages(server, local);
    expect(merged.some((m) => m.id === "ws-stream-sess-1")).toBe(true);
    expect(merged.some((m) => m.id === "act-tc-1")).toBe(true);
  });

  it("sorts by turn_index before created_at", () => {
    const server = [
      { ...msg("a", "ok", "2026-05-20T10:00:02Z"), turn_index: 2 },
      { ...msg("b", "ok", "2026-05-20T10:00:01Z"), turn_index: 1 },
    ];
    const local = [{ ...msg("ws-stream-sess-1", "streaming", "2026-05-20T10:00:03Z"), turn_index: 3 }];
    const merged = mergeSessionMessages(server, local);
    expect(merged.map((m) => m.id)).toEqual(["b", "a", "ws-stream-sess-1"]);
  });

  it("detects chat.activity schema", () => {
    const row = msg("act-tc-2");
    row.options_json = JSON.stringify({ schema: "chat.activity/v1", tool_event: { id: "tc-2" } });
    expect(isActivityMessage(row)).toBe(true);
  });
});
