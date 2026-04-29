import { createAvatarService } from "../../services";
import { requestHandler } from "../../services/axiosHandler";

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
