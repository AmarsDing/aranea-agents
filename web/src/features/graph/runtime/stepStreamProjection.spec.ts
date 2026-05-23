import { describe, expect, it } from "vitest";
import { upsertStepFromStreamEvent } from "./stepStreamProjection";

describe("upsertStepFromStreamEvent", () => {
  it("appends and sorts steps by index", () => {
    const first = upsertStepFromStreamEvent([], {
      nodeId: "a",
      stepIndex: 2,
      status: "completed",
    });
    const second = upsertStepFromStreamEvent(first, {
      nodeId: "b",
      stepIndex: 1,
      status: "running",
    });
    expect(second.map((step) => step.nodeId)).toEqual(["b", "a"]);
  });

  it("updates an existing step in place", () => {
    const seeded = upsertStepFromStreamEvent([], {
      nodeId: "a",
      stepIndex: 1,
      status: "running",
    });
    const updated = upsertStepFromStreamEvent(seeded, {
      nodeId: "a",
      stepIndex: 1,
      status: "completed",
      error: "",
    });
    expect(updated).toHaveLength(1);
    expect(updated[0]?.status).toBe("completed");
  });
});
