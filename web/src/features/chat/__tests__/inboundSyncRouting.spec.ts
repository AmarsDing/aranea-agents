import { describe, expect, it } from "vitest";
import type { Envelope } from "../envelope";
import { SESSION_RUN_STATUS } from "../sessionRunStatus";
import {
  shouldGlobalHubFinalizeTurn,
  shouldGlobalHubHandleStream,
  shouldScheduleChannelFocus,
  shouldSessionWsSkipEnvelope,
  shouldSkipMessageReloadOnChannelFocus,
} from "../inboundSyncRouting";

function env(partial: Partial<Envelope>): Envelope {
  return {
    id: "e1",
    type: "text_delta",
    author: "test",
    session_id: "sess-1",
    timestamp: "",
    version: 1,
    ...partial,
  };
}

describe("inboundSyncRouting (DECO-R-P1)", () => {
  it("global hub handles channel stream only", () => {
    const stream = env({ type: "text_delta", source: "channel" });
    expect(shouldGlobalHubHandleStream(true, "agent", stream)).toBe(true);
    expect(shouldGlobalHubHandleStream(false, "agent", stream)).toBe(false);
    expect(shouldGlobalHubHandleStream(true, "team", stream)).toBe(false);
  });

  it("session WS skips channel-owned envelopes", () => {
    expect(shouldSessionWsSkipEnvelope(env({ source: "channel" }))).toBe(true);
    expect(shouldSessionWsSkipEnvelope(env({ source: "web" }))).toBe(false);
    expect(
      shouldSessionWsSkipEnvelope(env({ metadata: { source: "channel" } }))
    ).toBe(true);
  });

  it("global hub finalizeTurn for channel or background sessions", () => {
    expect(shouldGlobalHubFinalizeTurn(true, true, true)).toBe(true);
    expect(shouldGlobalHubFinalizeTurn(true, false, true)).toBe(true);
    expect(shouldGlobalHubFinalizeTurn(false, false, true)).toBe(true);
    expect(shouldGlobalHubFinalizeTurn(false, true, true)).toBe(false);
    expect(shouldGlobalHubFinalizeTurn(false, true, false)).toBe(false);
  });

  it("schedules channel focus only when auto-focus preconditions hold", () => {
    expect(
      shouldScheduleChannelFocus({
        channelInbound: true,
        channelAgentId: "agent-1",
        focusTrigger: true,
        isChatRoute: true,
        isViewingSession: false,
        shouldAutoFocus: true,
        hasFocusHandler: true,
      })
    ).toBe(true);
    expect(
      shouldScheduleChannelFocus({
        channelInbound: true,
        channelAgentId: "agent-1",
        focusTrigger: true,
        isChatRoute: true,
        isViewingSession: true,
        shouldAutoFocus: true,
        hasFocusHandler: true,
      })
    ).toBe(false);
  });

  it("skips message reload on RUNNING channel focus (DECO-R-P2-01)", () => {
    expect(shouldSkipMessageReloadOnChannelFocus(SESSION_RUN_STATUS.RUNNING)).toBe(true);
    expect(shouldSkipMessageReloadOnChannelFocus(SESSION_RUN_STATUS.COMPLETED)).toBe(false);
  });
});
