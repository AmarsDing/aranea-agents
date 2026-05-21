import { describe, expect, it } from "vitest";
import { agentNeedsSettingsHydration } from "../agentPlannerSettings";
import type { Agent } from "../../agents/types";

describe("agentNeedsSettingsHydration", () => {
  const base = { id: "a1", agent_key: "k", display_name: "A" } as Agent;

  it("requires hydration when settings missing", () => {
    expect(agentNeedsSettingsHydration({ ...base, settings: undefined })).toBe(true);
  });

  it("requires hydration when planner_kind empty", () => {
    expect(agentNeedsSettingsHydration({ ...base, settings: { planner_kind: "" } })).toBe(true);
  });

  it("skips hydration when planner_kind set", () => {
    expect(agentNeedsSettingsHydration({ ...base, settings: { planner_kind: "react" } })).toBe(false);
  });
});
