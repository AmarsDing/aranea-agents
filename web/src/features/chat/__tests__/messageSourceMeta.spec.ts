import { describe, expect, it } from "vitest";
import {
  messageSourceChipKey,
  parseMessageSourceMeta,
} from "../messageSourceMeta";

describe("messageSourceMeta", () => {
  it("parses channel source with platform", () => {
    const meta = parseMessageSourceMeta(
      JSON.stringify({ source: "channel", platform: "feishu" })
    );
    expect(meta?.source).toBe("channel");
    expect(meta?.platform).toBe("feishu");
    expect(messageSourceChipKey(meta)).toBe("chat.source.feishu");
  });

  it("returns null without source", () => {
    expect(parseMessageSourceMeta("{}")).toBeNull();
  });
});
