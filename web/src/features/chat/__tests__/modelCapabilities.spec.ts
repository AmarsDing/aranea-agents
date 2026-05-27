import { describe, expect, it } from "vitest";
import {
  isLikelyImageAttachmentName,
  modelSupportsImageInput,
  shouldBlockImageAttachmentsForModel,
} from "../modelCapabilities";

describe("chat model capabilities", () => {
  it("detects image attachment names", () => {
    expect(isLikelyImageAttachmentName("photo.PNG")).toBe(true);
    expect(isLikelyImageAttachmentName("notes.pdf")).toBe(false);
  });

  it("treats DeepSeek-like models as text-only for images", () => {
    expect(modelSupportsImageInput({ provider: "deepseek", model: "v4" })).toBe(false);
    expect(modelSupportsImageInput({ provider: "openai", model: "gpt-4o" })).toBe(true);
  });

  it("lets explicit vision capability override provider heuristics", () => {
    expect(modelSupportsImageInput({ provider: "deepseek", model: "vision", capabilities: { vision: true } })).toBe(true);
  });

  it("honors explicit non-vision capability metadata", () => {
    expect(modelSupportsImageInput({ provider: "other", model: "text", capabilities: { vision: false } })).toBe(false);
    expect(modelSupportsImageInput({ provider: "other", model: "text", capabilities: { vision: true, text_only: true } })).toBe(false);
  });

  it("blocks image attachments only for text-only models", () => {
    const attachments = [{ id: "a1", name: "image.png", progress: 1 }];
    expect(shouldBlockImageAttachmentsForModel({ provider: "deepseek", model: "v4" }, attachments)).toBe(true);
    expect(shouldBlockImageAttachmentsForModel({ provider: "openai", model: "gpt-4o" }, attachments)).toBe(false);
  });

  it("uses attachment mime type when filename has no image extension", () => {
    const attachments = [{ id: "a1", name: "upload", mime_type: "image/png", progress: 1 }];
    expect(shouldBlockImageAttachmentsForModel({ provider: "deepseek", model: "v4" }, attachments)).toBe(true);
  });
});
