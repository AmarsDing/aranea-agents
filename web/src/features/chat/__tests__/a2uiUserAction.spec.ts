import { describe, expect, it } from "vitest";
import { buildUserActionPayload, formatUserActionMessage } from "../a2uiUserAction";

describe("a2uiUserAction", () => {
  it("formats single JSONL userAction line for WS", () => {
    const payload = buildUserActionPayload({
      name: "submit",
      surfaceId: "s1",
      sourceComponentId: "btn-1",
      context: { orderId: "42" },
    });
    expect(payload.name).toBe("submit");
    const line = formatUserActionMessage(payload);
    const parsed = JSON.parse(line) as { userAction: typeof payload };
    expect(parsed.userAction.surfaceId).toBe("s1");
    expect(parsed.userAction.context.orderId).toBe("42");
    expect(line.includes("\n")).toBe(false);
  });
});
