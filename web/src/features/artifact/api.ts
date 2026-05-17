/**
 * Artifact 制品：**`createArtifactService()`** → **`/v1/artifacts`**。
 *
 * 注意：后端 Artifact 运行时/S3 后端尚在完善（EP-RT-08）。
 * 在生产存储未配置时，写接口可能返回错误，前端应做友好提示。
 */
import { createArtifactService } from "../../services";
import { asRecord, pickI32, pickNum, pickStr } from "../../shared/wireJson";
import type {
  ArtifactData,
  ArtifactMeta,
  ListArtifactsParams,
  ListArtifactsResult,
  UploadArtifactInput
} from "./types";

const svc = createArtifactService();

function mapMeta(raw: unknown): ArtifactMeta {
  const r = asRecord(raw);
  return {
    id: pickStr(r, "id", "id"),
    session_id: pickStr(r, "session_id", "sessionId"),
    name: pickStr(r, "name", "name"),
    mime_type: pickStr(r, "mime_type", "mimeType"),
    size: pickNum(r, "size", "size"),
    sha256: pickStr(r, "sha256", "sha256"),
    storage_kind: pickStr(r, "storage_kind", "storageKind"),
    storage_uri: pickStr(r, "storage_uri", "storageUri"),
    version: pickI32(r, "version", "version"),
    created_at: pickStr(r, "created_at", "createdAt")
  };
}

function mapData(raw: unknown): ArtifactData {
  const r = asRecord(raw);
  const metaRaw = r.meta ?? r.Meta;
  return {
    meta: mapMeta(metaRaw),
    data_base64: pickStr(r, "data_base64", "dataBase64")
  };
}

export async function listArtifacts(params: ListArtifactsParams = {}): Promise<ListArtifactsResult> {
  const res = asRecord(
    await svc.ListArtifacts({
      sessionId: params.session_id ?? "",
      limit: params.limit ?? 0,
      offset: params.offset ?? 0
    })
  );
  const itemsRaw = res.items ?? res.Items;
  const items = Array.isArray(itemsRaw) ? itemsRaw.map(mapMeta) : [];
  return { items, total: pickI32(res, "total", "total") || items.length };
}

export async function getArtifact(id: string, version?: number): Promise<ArtifactData> {
  const raw = await svc.GetArtifact({ id, version: version ?? 0 });
  return mapData(raw);
}

export async function uploadArtifact(input: UploadArtifactInput): Promise<ArtifactMeta> {
  const raw = await svc.UploadArtifact({
    sessionId: input.session_id,
    name: input.name,
    mimeType: input.mime_type ?? "",
    dataBase64: input.data_base64
  });
  return mapMeta(raw);
}

export async function deleteArtifact(id: string): Promise<void> {
  await svc.DeleteArtifact({ id });
}
