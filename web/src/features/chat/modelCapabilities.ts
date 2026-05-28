import type { ChatAttachment } from "../../components/chat/types";

export type ChatModelDescriptor = {
  provider?: string;
  model?: string;
  capabilities?: {
    vision?: boolean;
    image?: boolean;
    text_only?: boolean;
    file?: boolean;
  };
};

export function isLikelyImageAttachmentName(name: string): boolean {
  return /\.(png|jpe?g|gif|webp|bmp|svg|heic|heif)$/i.test(name || "");
}

export function modelSupportsImageInput(model: ChatModelDescriptor): boolean {
  if (model.capabilities && Object.keys(model.capabilities).length > 0) {
    return (model.capabilities.vision === true || model.capabilities.image === true) && model.capabilities.text_only !== true;
  }
  const key = `${model.provider ?? ""} ${model.model ?? ""}`.toLowerCase();
  if (key.includes("deepseek")) {
    return false;
  }
  return true;
}

export function modelSupportsFileInput(model: ChatModelDescriptor): boolean {
  if (model.capabilities && Object.keys(model.capabilities).length > 0) {
    if (model.capabilities.text_only === true) return false;
    return model.capabilities.file !== false;
  }
  return true;
}

/** Non-image file extensions for models that support files but not vision. */
const NON_IMAGE_EXTENSIONS =
  ".pdf,.txt,.doc,.docx,.csv,.xlsx,.md,.json,.xml,.html,.py,.js,.ts,.go,.java,.c,.cpp,.h,.rs,.rb,.php,.sh,.yaml,.yml,.toml,.ini,.cfg,.log,.sql,.r,.tex";

/**
 * Returns the `accept` attribute value for the file input element.
 * - Empty string → accept all types (model supports both files and images)
 * - Extension list → accept only non-image files (model supports files but not images)
 * - Empty string + button disabled → no files accepted (model does not support files)
 */
export function fileAcceptForModel(model: ChatModelDescriptor): string {
  if (!modelSupportsFileInput(model)) return "";
  if (!modelSupportsImageInput(model)) return NON_IMAGE_EXTENSIONS;
  return "";
}

export type AttachmentBlockReason = "" | "ATTACHMENT_UNSUPPORTED" | "IMAGE_UNSUPPORTED";

/**
 * Unified check: should the current attachments be blocked for this model?
 * Returns the reason string (empty = not blocked).
 */
export function shouldBlockAttachmentsForModel(
  model: ChatModelDescriptor,
  attachments: ChatAttachment[]
): AttachmentBlockReason {
  if (!modelSupportsFileInput(model) && attachments.length > 0) {
    return "ATTACHMENT_UNSUPPORTED";
  }
  if (!modelSupportsImageInput(model) && attachments.some(isImageAttachment)) {
    return "IMAGE_UNSUPPORTED";
  }
  return "";
}

/** @deprecated Use shouldBlockAttachmentsForModel instead. */
export function shouldBlockImageAttachmentsForModel(
  model: ChatModelDescriptor,
  attachments: ChatAttachment[]
): boolean {
  return shouldBlockAttachmentsForModel(model, attachments) === "IMAGE_UNSUPPORTED";
}

function isImageAttachment(item: ChatAttachment): boolean {
  return item.mime_type?.toLowerCase().startsWith("image/") || isLikelyImageAttachmentName(item.name);
}

/** Check if a clipboard file (by type/name) is acceptable for the model. */
export function isClipboardFileAcceptableForModel(
  model: ChatModelDescriptor,
  file: { type?: string; name?: string }
): boolean {
  if (!modelSupportsFileInput(model)) return false;
  const isImage = (file.type?.toLowerCase().startsWith("image/") || isLikelyImageAttachmentName(file.name ?? ""));
  if (isImage && !modelSupportsImageInput(model)) return false;
  return true;
}
