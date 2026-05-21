import { describe, expect, it } from "vitest";
import { parseReactPlannerContent, shouldUseReactPlannerView } from "../reactPlannerParse";

describe("reactPlannerParse", () => {
  it("splits planning and final answer", () => {
    const text = `/*PLANNING*/\nStep one plan.\n/*FINAL_ANSWER*/\nThe answer is 42.`;
    const parsed = parseReactPlannerContent(text);
    expect(parsed).not.toBeNull();
    expect(parsed!.steps).toHaveLength(1);
    expect(parsed!.steps[0].kind).toBe("planning");
    expect(parsed!.finalAnswer).toContain("42");
  });

  it("enables view for react planner kind", () => {
    expect(shouldUseReactPlannerView("react", "hello")).toBe(true);
    expect(shouldUseReactPlannerView("", "/*PLANNING*/ x")).toBe(true);
    expect(shouldUseReactPlannerView("a2ui", "/*PLANNING*/ x")).toBe(false);
  });
});
