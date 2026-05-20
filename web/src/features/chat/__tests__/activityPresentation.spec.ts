import { describe, expect, it } from "vitest";
import { maskSensitiveJSON } from "../activityPresentation";

describe("activityPresentation", () => {
  it("masks sensitive json keys", () => {
    const masked = maskSensitiveJSON({
      path: "/tmp/a.txt",
      api_key: "secret123",
      nested: { token: "abc" },
    }) as Record<string, unknown>;
    expect(masked.path).toBe("/tmp/a.txt");
    expect(masked.api_key).toBe("***");
    expect((masked.nested as Record<string, unknown>).token).toBe("***");
  });
});
