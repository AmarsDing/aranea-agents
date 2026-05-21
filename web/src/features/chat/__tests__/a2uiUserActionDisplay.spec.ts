import { describe, expect, it } from "vitest";
import {
  formatUserActionUserMarkdown,
  parseUserActionFromContent,
} from "../a2uiUserActionDisplay";

describe("a2uiUserActionDisplay", () => {
  it("parses userAction JSON line", () => {
    const line = JSON.stringify({
      userAction: {
        name: "submit",
        surfaceId: "s1",
        sourceComponentId: "btn",
        timestamp: "2026-05-21T00:00:00Z",
        context: { x: 1 },
      },
    });
    const ua = parseUserActionFromContent(line);
    expect(ua?.name).toBe("submit");
    expect(formatUserActionUserMarkdown(ua!)).toContain("submit");
  });
});
