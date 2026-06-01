import axios from "axios";
import { createAvatarService } from "../../services";
import { requestHandler } from "../../services/axiosHandler";

/** 与后端 `internal/biz/avatar.go` 一致 */
export const AVATAR_MAX_FILE_BYTES = 2 * 1024 * 1024;

const AVATAR_ALLOWED_MIME = new Set(["image/png", "image/jpeg", "image/webp"]);

/** 上传前校验；通过返回 `null`，否则返回简短中文错误说明 */
export function validateAvatarFileForUpload(file: File): string | null {
  if (!file?.size) {
    return "请选择有效的图片文件";
  }
  if (file.size > AVATAR_MAX_FILE_BYTES) {
    return "头像图片须不超过 2MB";
  }
  const mime = (file.type || "").toLowerCase();
  if (!AVATAR_ALLOWED_MIME.has(mime)) {
    return "仅支持 PNG、JPEG、WebP 格式";
  }
  return null;
}

/** 将 axios / 服务端错误整理为可展示文案 */
export function avatarUploadErrorMessage(err: unknown): string {
  if (axios.isAxiosError(err)) {
    const data = err.response?.data;
    if (data && typeof data === "object" && !Array.isArray(data)) {
      const o = data as Record<string, unknown>;
      const raw = o.message ?? o.msg;
      if (typeof raw === "string" && raw.trim()) {
        return mapAvatarBackendMessage(raw.trim());
      }
    }
    const status = err.response?.status;
    if (status === 400) {
      return "图片不符合要求，请检查格式（PNG / JPEG / WebP）与大小（≤ 2MB）";
    }
    if (status === 401 || status === 403) {
      return "未登录或无权上传头像";
    }
    if (status === 413) {
      return "头像图片须不超过 2MB";
    }
  }
  if (err instanceof Error && err.message) {
    return err.message;
  }
  return "头像上传失败";
}

function mapAvatarBackendMessage(message: string): string {
  const table: Record<string, string> = {
    "avatar file is required": "请选择有效的图片文件",
    "avatar file must be <= 2MB": "头像图片须不超过 2MB",
    "unsupported avatar type": "仅支持 PNG、JPEG、WebP 格式"
  };
  if (table[message]) {
    return table[message];
  }
  if (message.startsWith("unsupported avatar type:")) {
    return "仅支持 PNG、JPEG、WebP 格式";
  }
  return message;
}

export type AvatarAsset = {
  id: string;
  key: string;
  name: string;
  description: string;
  mime_type: string;
  workspace_id: string;
  owner_user_id: string;
  source: "system" | "upload";
  is_system: boolean;
  category: "agent" | "channel";
  file_size_bytes: number;
  width_px: number;
  height_px: number;
  sort_order: number;
  created_at: string;
};

function mapSvcAvatarRow(row: unknown): AvatarAsset {
  const r = row as Record<string, unknown>;
  const s = (snake: string, camel: string) => String(r[snake] ?? r[camel] ?? "");
  const n = (snake: string, camel: string) => Number(r[snake] ?? r[camel] ?? 0);
  const b = (snake: string, camel: string) => Boolean(r[snake] ?? r[camel]);
  const src = s("source", "source");
  return {
    id: s("id", "id"),
    key: s("key", "key"),
    name: s("name", "name"),
    description: s("description", "description"),
    mime_type: s("mime_type", "mimeType"),
    workspace_id: s("workspace_id", "workspaceId"),
    owner_user_id: s("owner_user_id", "ownerUserId"),
    source: src === "system" || src === "upload" ? src : "upload",
    is_system: b("is_system", "isSystem"),
    category: (s("category", "category") === "channel" ? "channel" : "agent") as "agent" | "channel",
    file_size_bytes: n("file_size_bytes", "fileSizeBytes"),
    width_px: n("width_px", "widthPx"),
    height_px: n("height_px", "heightPx"),
    sort_order: n("sort_order", "sortOrder"),
    created_at: s("created_at", "createdAt")
  };
}

function fileToBase64Payload(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const s = reader.result as string;
      const i = s.indexOf(",");
      resolve(i >= 0 ? s.slice(i + 1) : s);
    };
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

export async function listAvatarAssets(scope?: "system" | "mine"): Promise<AvatarAsset[]> {
  const svc = createAvatarService();
  const res = (await svc.ListAvatarAssets({
    scope: scope ?? undefined,
    workspaceId: undefined,
    ownerUserId: undefined
  })) as { items?: unknown[] };
  return (res.items ?? []).map((row) => mapSvcAvatarRow(row));
}

export async function uploadAvatarAsset(file: File): Promise<AvatarAsset> {
  const image_data = await fileToBase64Payload(file);
  const body = JSON.stringify({
    filename: file.name || undefined,
    image_data
  });
  const raw = await requestHandler({
    path: "v1/avatar-assets",
    method: "POST",
    body
  });
  return mapSvcAvatarRow(raw);
}

/** Kratos JSON 缩略图 → data URL（不含缓存；缓存由 Store 管理） */
export async function getAvatarThumbnailDataUrl(assetId: string): Promise<string> {
  const trimmed = String(assetId || "").trim();
  if (!trimmed) return "";
  const svc = createAvatarService();
  const res = (await svc.GetAvatarThumbnail({ id: trimmed })) as Record<string, unknown>;
  const mime = String(res.mimeType ?? res.mime_type ?? "image/png");
  const b64 = res.data;
  if (typeof b64 !== "string" || !b64) return "";
  return `data:${mime};base64,${b64}`;
}
