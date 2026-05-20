import { describe, it, expect } from "vitest";
import type { Envelope } from "../../chat/envelope";
import {
  buildFlowDiagnosticJsonl,
  flowLogExportEntryFromLine,
  flowLogMatchesTrace,
  monitorLogLineFromFlowEnvelope,
  traceCorrelationFromUsageRow
} from "../flow";
import type { MonitorLogLine, MonitorTraceEvent } from "../types";

describe("monitorLogLineFromFlowEnvelope", () => {
  it("maps flow_log metadata to MonitorLogLine", () => {
    const env: Envelope = {
      id: "env-1",
      type: "flow_log",
      timestamp: "2026-05-20T12:00:00.000Z",
      session_id: "sess-1",
      author: "flow",
      channel: "monitor",
      metadata: {
        schema_version: "flow_log/v1",
        flow_id: "fl_abc",
        trace_id: "tr_123",
        run_id: "run_456",
        step_id: "chat.llm.invoke",
        flow_phase: "done",
        severity: "ok",
        title: "调用语言模型",
        message: "模型已返回（120ms）"
      },
      content: { text: "调用语言模型 — 模型已返回（120ms）", is_partial: false }
    };
    const line = monitorLogLineFromFlowEnvelope(env);
    expect(line).not.toBeNull();
    expect(line?.kind).toBe("flow");
    expect(line?.severity).toBe("ok");
    expect(line?.title).toBe("调用语言模型");
    expect(line?.trace_id).toBe("tr_123");
    expect(line?.run_id).toBe("run_456");
    expect(line?.level).toBe("INFO");
  });
});

describe("flowLogMatchesTrace", () => {
  const flowLine: MonitorLogLine = {
    id: "fl_1",
    time: "2026-05-20T12:00:00Z",
    level: "INFO",
    message: "ok",
    source: "agent",
    created_at: "2026-05-20T12:00:00Z",
    kind: "flow",
    trace_id: "tr_a",
    run_id: "run_b",
    session_id: "sess_c"
  };

  it("matches when trace and run align", () => {
    expect(flowLogMatchesTrace(flowLine, { traceId: "tr_a", runId: "run_b" })).toBe(true);
  });

  it("rejects mismatched trace_id", () => {
    expect(flowLogMatchesTrace(flowLine, { traceId: "tr_other" })).toBe(false);
  });

  it("ignores non-flow lines", () => {
    expect(flowLogMatchesTrace({ ...flowLine, kind: "process" }, { traceId: "tr_a" })).toBe(false);
  });
});

describe("traceCorrelationFromUsageRow", () => {
  it("reads trace_id from metadata_json", () => {
    const row = {
      message_id: "run-1",
      session_id: "sess-9",
      metadata_json: JSON.stringify({ trace_id: "tr_meta", spans: [] })
    } as MonitorTraceEvent;
    expect(traceCorrelationFromUsageRow(row)).toEqual({
      traceId: "tr_meta",
      runId: "run-1",
      sessionId: "sess-9"
    });
  });
});

describe("buildFlowDiagnosticJsonl", () => {
  it("emits bundle header and entries", () => {
    const line = flowLogExportEntryFromLine({
      id: "fl_x",
      time: "2026-05-20T12:00:01Z",
      level: "ERROR",
      message: "失败",
      source: "chat",
      created_at: "2026-05-20T12:00:01Z",
      kind: "flow",
      severity: "error",
      title: "对话失败",
      trace_id: "tr_exp",
      step_id: "chat.turn.timeout"
    });
    const jsonl = buildFlowDiagnosticJsonl("tr_exp", [
      {
        id: "fl_x",
        time: "2026-05-20T12:00:01Z",
        level: "ERROR",
        message: "失败",
        source: "chat",
        created_at: "2026-05-20T12:00:01Z",
        kind: "flow",
        severity: "error",
        title: "对话失败",
        trace_id: "tr_exp",
        step_id: "chat.turn.timeout"
      }
    ]);
    const lines = jsonl.trim().split("\n");
    expect(lines).toHaveLength(2);
    const header = JSON.parse(lines[0]) as { type: string; entry_count: number };
    expect(header.type).toBe("flow_diagnostic_bundle");
    expect(header.entry_count).toBe(1);
    expect(JSON.parse(lines[1])).toEqual(line);
  });
});
