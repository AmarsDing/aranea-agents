/**
 * deliverables 通知载荷解析（P1 会话产物点击查看，2026-09-02）。
 *
 * 后端在全部团队完成后发布 orphan notice step（NoticeType=deliverables，
 * Content 为 {"artifacts":[{artifact_id,name,format,size_chars,mime_type}]}
 * JSON 载荷，见 internal/service/spirit_team.go publishDeliverablesNotice）。
 * NoticeBlock 用本模块把载荷解析为产物卡片列表；解析失败返回 null，
 * 调用方退化回普通 markdown 通知渲染。
 */
import { asArray, asRecord, asString, tryParseJson } from '../../components/chat/tools/toolDetailShared';

export const DELIVERABLES_NOTICE_TYPE = 'deliverables';

export type DeliverableRef = {
  artifactId: string;
  name?: string;
  mimeType?: string;
};

/** 解析 deliverables 通知载荷；非该类型 / 解析失败 / 无有效条目时返回 null。 */
export function parseDeliverableRefs(noticeType: string | undefined, content: string): DeliverableRef[] | null {
  if ((noticeType ?? '').trim().toLowerCase() !== DELIVERABLES_NOTICE_TYPE) return null;
  const payload = asRecord(tryParseJson(content));
  const rawItems = asArray(payload?.artifacts);
  if (!payload || !rawItems) return null;
  const refs: DeliverableRef[] = [];
  for (const item of rawItems) {
    const rec = asRecord(item);
    const artifactId = asString(rec?.artifact_id)?.trim();
    if (!artifactId) continue;
    refs.push({
      artifactId,
      name: asString(rec?.name)?.trim() || undefined,
      mimeType: asString(rec?.mime_type)?.trim() || formatToMimeHint(asString(rec?.format)),
    });
  }
  return refs.length > 0 ? refs : null;
}

/** 产物 format（如 markdown/pdf/png）→ MIME 提示，供卡片选图标。 */
export function formatToMimeHint(format?: string): string | undefined {
  const f = (format ?? '').trim().toLowerCase();
  if (!f) return undefined;
  if (f === 'pdf') return 'application/pdf';
  if (f === 'json') return 'application/json';
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg'].includes(f)) return `image/${f}`;
  if (['mp4', 'webm', 'mov'].includes(f)) return `video/${f}`;
  if (['mp3', 'wav', 'ogg', 'flac'].includes(f)) return `audio/${f}`;
  // markdown/html/txt/docx/pptx 等统一按文本类图标展示。
  return 'text/plain';
}
