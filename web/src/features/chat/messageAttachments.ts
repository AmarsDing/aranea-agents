/** Parsed artifact attachment refs from user message options_json (ART-01). */
export type MessageAttachmentRef = {
  id: string;
  name: string;
  mime_type: string;
  size?: number;
};

export function parseMessageAttachments(optionsJson: string | undefined): MessageAttachmentRef[] {
  const raw = optionsJson?.trim();
  if (!raw) return [];
  try {
    const opts = JSON.parse(raw) as { attachments?: unknown };
    if (!Array.isArray(opts.attachments)) return [];
    const out: MessageAttachmentRef[] = [];
    for (const item of opts.attachments) {
      if (!item || typeof item !== "object") continue;
      const rec = item as Record<string, unknown>;
      const id = typeof rec.id === "string" ? rec.id.trim() : "";
      if (!id) continue;
      out.push({
        id,
        name: typeof rec.name === "string" ? rec.name : id,
        mime_type: typeof rec.mime_type === "string" ? rec.mime_type : "application/octet-stream",
        size: typeof rec.size === "number" ? rec.size : undefined,
      });
    }
    return out;
  } catch {
    return [];
  }
}

export function attachmentMimeIcon(mime: string): string {
  const m = (mime || "").toLowerCase();
  if (m.startsWith("image/")) return "image";
  if (m === "application/pdf") return "picture_as_pdf";
  if (m.startsWith("text/") || m.includes("json") || m.includes("xml") || m.includes("javascript")) return "code";
  if (m.startsWith("video/")) return "videocam";
  if (m.startsWith("audio/")) return "audiotrack";
  return "insert_drive_file";
}

export function formatAttachmentBytes(n?: number): string {
  if (!n) return "";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}
