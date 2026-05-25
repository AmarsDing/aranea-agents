import { describe, expect, it } from "vitest";
import { isCatalogJsonBlock, normalizeCatalogSearchBlocks } from "../catalogSearchUtils";

describe("catalogSearchUtils", () => {
  it("accepts complete JSON objects", () => {
    expect(isCatalogJsonBlock('{"id":"deepseek"}')).toBe(true);
  });

  it("rejects line fragments", () => {
    expect(isCatalogJsonBlock('"deepseek-chat": {')).toBe(false);
  });

  it("detects legacy line mode", () => {
    const res = normalizeCatalogSearchBlocks(['"id": "x",', '"name": "y"']);
    expect(res.legacyLineMode).toBe(true);
    expect(res.blocks).toEqual([]);
  });

  it("passes valid blocks through", () => {
    const block = '{\n  "id": "deepseek"\n}';
    const res = normalizeCatalogSearchBlocks([block]);
    expect(res.legacyLineMode).toBe(false);
    expect(res.blocks).toEqual([block]);
  });
});
