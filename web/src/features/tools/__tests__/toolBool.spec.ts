import { describe, expect, it } from "vitest";
import { toolBool } from "../api";

describe("toolBool", () => {
  it("preserves boolean values", () => {
    expect(toolBool(true)).toBe(true);
    expect(toolBool(false)).toBe(false);
  });

  it("parses string booleans from legacy form state", () => {
    expect(toolBool("true")).toBe(true);
    expect(toolBool("false")).toBe(false);
    expect(toolBool("TRUE")).toBe(true);
  });

  it("treats other values as falsey except true strings", () => {
    expect(toolBool("")).toBe(false);
    expect(toolBool(0)).toBe(false);
    expect(toolBool(1)).toBe(true);
  });
});
