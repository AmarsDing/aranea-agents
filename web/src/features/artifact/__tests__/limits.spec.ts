import { describe, expect, it } from "vitest";
import { ARTIFACT_MAX_BYTES, validateArtifactFileSize } from "../limits";

describe("artifact limits", () => {
  it("accepts files at limit", () => {
    expect(validateArtifactFileSize(ARTIFACT_MAX_BYTES)).toBeNull();
  });

  it("rejects files over limit", () => {
    expect(validateArtifactFileSize(ARTIFACT_MAX_BYTES + 1)).toMatch(/10 MB/);
  });
});
