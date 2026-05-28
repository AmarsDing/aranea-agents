import { describe, expect, it } from "vitest";
import {
  collectToolEventsAfterMessage,
  enrichReactStepsWithToolEvents,
  extractToolNamesFromActionBody,
} from "../reactPlannerToolLink";
import { buildReactToolLinkIndex, isToolLinkedInReactIndex } from "../reactToolLinkIndex";
import type { Message } from "../types";

function toolMessage(id: string, toolName: string, createdAt: string): Message {
  return {
    id,
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
        id: `tc-${id}`,
        phase: "after",
        status: "success",
        agent_id: "a1",
        agent_key: "agent",
        agent_name: "Agent",
        agent_icon: "",
        tool_name: toolName,
        tool_label: toolName,
        occurred_at: createdAt,
      },
    }),
    error_message: "",
    created_at: createdAt,
  };
}

describe("reactPlannerToolLink", () => {
  it("extracts functions.* tool hints", () => {
    expect(extractToolNamesFromActionBody("I will call functions.web_search now.")).toContain(
      "web_search"
    );
  });

  it("links ACTION step to following tool_call row", () => {
    const messages: Message[] = [
      {
        id: "m1",
        session_id: "s1",
        parent_message_id: "",
        turn_id: "",
        turn_number: 1,
        seq_in_turn: 0,
        role: "assistant",
        content_markdown: "/*ACTION*/\nUse functions.read_file.",
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
      toolMessage("act1", "read_file", "2026-05-21T10:00:01Z"),
    ];
    const pool = collectToolEventsAfterMessage(messages, 0);
    expect(pool).toHaveLength(1);
    expect(pool[0].tool_name).toBe("read_file");

    const enriched = enrichReactStepsWithToolEvents(
      [{ kind: "action", title: "动作", body: "functions.read_file" }],
      0,
      messages
    );
    expect(enriched[0].linkedTools).toHaveLength(1);
    expect(enriched[0].linkedTools[0].tool_name).toBe("read_file");
  });

  it("suppresses standalone tool via session index only", () => {
    const messages: Message[] = [
      {
        id: "m1",
        session_id: "s1",
        parent_message_id: "",
        turn_id: "",
        turn_number: 1,
        seq_in_turn: 0,
        role: "assistant",
        content_markdown: "/*ACTION*/\nfunctions.read_file",
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
      toolMessage("act1", "read_file", "2026-05-21T10:00:01Z"),
    ];
    const index = buildReactToolLinkIndex(messages);
    expect(isToolLinkedInReactIndex(index, "tc-act1")).toBe(true);
    expect(isToolLinkedInReactIndex(index, "tc-other")).toBe(false);
  });
});
