import { describe, expect, it } from "vitest";
import { formatJsonText, highlightJsonHtml } from "../jsonFormat";

describe("jsonFormat", () => {
  it("pretty-prints valid JSON", () => {
    expect(formatJsonText('{"a":1}')).toBe('{\n  "a": 1\n}');
  });

  it("returns raw text when parse fails", () => {
    expect(formatJsonText("not json")).toBe("not json");
  });

  it("highlights keys and strings", () => {
    const html = highlightJsonHtml('{\n  "id": "openai"\n}');
    expect(html).toContain('json-code-viewer__key');
    expect(html).toContain('json-code-viewer__string');
    expect(html).not.toContain("<script");
  });
});
