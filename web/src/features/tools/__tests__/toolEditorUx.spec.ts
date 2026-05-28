import { describe, expect, it } from "vitest";
import { editorTabForJsonKey, firstInvalidToolJsonKey } from "../../../components/tools/toolUi";
import { bindingSummaryLine } from "../toolAgentBindingSummary";

describe("editorTabForJsonKey", () => {
  it("maps schema fields to schema tab", () => {
    expect(editorTabForJsonKey("parameters_schema_json")).toBe("schema");
    expect(editorTabForJsonKey("config_json")).toBe("schema");
  });

  it("maps advanced fields to advanced tab", () => {
    expect(editorTabForJsonKey("default_config_json")).toBe("advanced");
    expect(editorTabForJsonKey("metadata_json")).toBe("advanced");
  });
});

describe("firstInvalidToolJsonKey", () => {
  it("returns first key in field order", () => {
    expect(
      firstInvalidToolJsonKey({
        metadata_json: "bad",
        parameters_schema_json: "bad"
      })
    ).toBe("parameters_schema_json");
  });

  it("returns null when no errors", () => {
    expect(firstInvalidToolJsonKey({})).toBeNull();
  });
});

describe("bindingSummaryLine", () => {
  it("formats allowed/denied and overrides", () => {
    const line = bindingSummaryLine({
      total_agents: 3,
      allowed: 2,
      denied: 1,
      tools_disabled_agents: 0,
      override_count: 1,
      rows: []
    });
    expect(line).toContain("2 个 Agent 可用");
    expect(line).toContain("1 条显式覆盖");
  });
});
