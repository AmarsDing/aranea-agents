import { describe, expect, it } from "vitest";
import { resolveA2UIKindRoute } from "../a2ui/a2uiKindRegistry";

describe("resolveA2UIKindRoute", () => {
  it("maps StandardCatalog kinds to routes", () => {
    expect(resolveA2UIKindRoute("Text")).toBe("primitive");
    expect(resolveA2UIKindRoute("Button")).toBe("form");
    expect(resolveA2UIKindRoute("List")).toBe("layout");
    expect(resolveA2UIKindRoute("Card")).toBe("container");
    expect(resolveA2UIKindRoute("Nope")).toBe("unknown");
  });
});
