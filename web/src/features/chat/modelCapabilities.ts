import type { ChatAttachment } from "../../components/chat/types";

export type ChatModelDescriptor = {
  provider?: string;
  model?: string;
  capabilities?: {
    vision?: boolean;
    image?: boolean;
    text_only?: boolean;
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

export function shouldBlockImageAttachmentsForModel(
  model: ChatModelDescriptor,
  attachments: ChatAttachment[]
): boolean {
  return !modelSupportsImageInput(model) && attachments.some((item) => isImageAttachment(item));
}

function isImageAttachment(item: ChatAttachment): boolean {
  return item.mime_type?.toLowerCase().startsWith("image/") || isLikelyImageAttachmentName(item.name);
}
