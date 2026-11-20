// 邮件页纯逻辑（设计 docs/aranea-agents/design/2026-11-17-三大模块重设计/09-邮件.md §3）。
// 地址解析、按日期分组、回复 / 转发预填、HTML 正文的 iframe 文档（CSP）都在这里，便于单测。

import type { MailAttachmentRow, MailMessageRow } from '../office/types';

export type MailAddr = { name: string; address: string };

/** 「李卓 <li@x.cn>」/「"李卓" <li@x.cn>」/「li@x.cn」→ { name, address }。 */
export function parseAddr(raw: string): MailAddr {
  const s = raw.trim();
  const m = /^(.*?)<([^>]+)>\s*$/.exec(s);
  if (m) {
    const name = (m[1] ?? '')
      .trim()
      .replace(/^"(.*)"$/, '$1')
      .trim();
    return { name, address: (m[2] ?? '').trim() };
  }
  return { name: '', address: s.replace(/^"(.*)"$/, '$1') };
}

/** 按逗号 / 分号（含全角）切地址，引号和尖括号里的不切。 */
export function splitAddrs(list: string): string[] {
  const out: string[] = [];
  let cur = '';
  let quote = false;
  let angle = false;
  for (const ch of list) {
    if (ch === '"') quote = !quote;
    else if (ch === '<') angle = true;
    else if (ch === '>') angle = false;
    if (!quote && !angle && (ch === ',' || ch === ';' || ch === '，' || ch === '；')) {
      if (cur.trim()) out.push(cur.trim());
      cur = '';
      continue;
    }
    cur += ch;
  }
  if (cur.trim()) out.push(cur.trim());
  return out;
}

/** 列表里显示的名字：有名字用名字，没有用地址 @ 前面那段。 */
export function displayName(raw: string): string {
  const a = parseAddr(raw);
  if (a.name) return a.name;
  return a.address.split('@')[0] || a.address || raw;
}

/** 头像字：名字第一个字（拉丁字母大写）。 */
export function initial(raw: string): string {
  const ch = [...displayName(raw).trim()][0] ?? '?';
  return ch.toUpperCase();
}

/** 同一发件人颜色固定：地址哈希到 1..5（对应 --nt-line-1..5）。 */
export function avatarTone(raw: string): number {
  const key = (parseAddr(raw).address || raw).toLowerCase();
  let h = 0;
  for (const ch of key) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return (h % 5) + 1;
}

export type DateGroupKey = 'today' | 'yesterday' | 'week' | 'earlier';

function startOfDay(d: Date): number {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

export function dateGroup(iso: string, now: Date): DateGroupKey {
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return 'earlier';
  const today = startOfDay(now);
  if (t >= today) return 'today';
  if (t >= today - 86_400_000) return 'yesterday';
  // 本周：周一开始
  const monday = today - ((now.getDay() + 6) % 7) * 86_400_000;
  return t >= monday ? 'week' : 'earlier';
}

/** 列表按「今天 / 昨天 / 本周 / 更早」分组（保持原有顺序）。 */
export function groupByDate(rows: MailMessageRow[], now: Date): { key: DateGroupKey; rows: MailMessageRow[] }[] {
  const out: { key: DateGroupKey; rows: MailMessageRow[] }[] = [];
  for (const r of rows) {
    const key = dateGroup(r.received_at, now);
    const last = out[out.length - 1];
    if (last && last.key === key) last.rows.push(r);
    else out.push({ key, rows: [r] });
  }
  return out;
}

const pad = (n: number) => String(n).padStart(2, '0');

/** 列表时间：今天 HH:mm；今年 MM-DD；更早 YYYY-MM-DD。 */
export function listTime(iso: string, now: Date): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  if (startOfDay(d) === startOfDay(now)) return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
  if (d.getFullYear() === now.getFullYear()) return `${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

export function fullTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export type ComposeKind = 'new' | 'reply' | 'replyAll' | 'forward' | 'draft';

/** 打开写信时带进来的上下文。 */
export type ComposeSeed = {
  kind: ComposeKind;
  /** 回复 / 转发的原信，或要继续写的草稿 */
  message?: MailMessageRow;
  /** 原信正文（引用用）或草稿正文 */
  body?: string;
  /** 转发：原信附件；草稿：草稿里的附件（都由服务端带上） */
  carried?: MailAttachmentRow[];
};

// 主题前缀：Re: / 回复： 和 Fwd: / 转发：（中文写成 \u 转义，免得被文案检查当成界面文字）
const REPLY_PREFIX = /^(re|\u56de\u590d)\s*[:\uff1a]/i;
const FORWARD_PREFIX = /^(fwd?|\u8f6c\u53d1)\s*[:\uff1a]/i;

/** 回复 / 转发主题：已经带了前缀就不再叠。 */
export function prefixedSubject(subject: string, kind: ComposeKind): string {
  const s = subject.trim();
  if (kind === 'reply' || kind === 'replyAll') return REPLY_PREFIX.test(s) ? s : `Re: ${s}`;
  if (kind === 'forward') return FORWARD_PREFIX.test(s) ? s : `Fwd: ${s}`;
  return s;
}

/** 引用原文（纯文本），附在新正文下面。 */
export function quotedOriginal(
  msg: MailMessageRow,
  body: string,
  labels: { header: string; from: string; to: string; time: string; subject: string },
): string {
  const head = [
    `—— ${labels.header} ——`,
    `${labels.from}: ${msg.sender}`,
    msg.to ? `${labels.to}: ${msg.to}` : '',
    `${labels.time}: ${fullTime(msg.received_at)}`,
    `${labels.subject}: ${msg.subject}`,
  ].filter(Boolean);
  const quoted = body
    .split('\n')
    .map((l) => `> ${l}`)
    .join('\n');
  return `\n\n${head.join('\n')}\n\n${quoted}`;
}

/** 回复 / 全部回复的收件人。全部回复：原发件人 + 原收件人 + 原抄送，去掉自己的地址、去重。 */
export function replyRecipients(msg: MailMessageRow, kind: ComposeKind, mine: string[]): { to: string; cc: string } {
  if (kind !== 'reply' && kind !== 'replyAll') return { to: '', cc: '' };
  // 回复自己发出的信 = 接着发给原收件人
  const own = new Set(mine.map((a) => a.toLowerCase()));
  const fromAddr = parseAddr(msg.sender).address.toLowerCase();
  const sentByMe = own.has(fromAddr) || msg.folder === 'sent';
  const seen = new Set<string>();
  const pick = (list: string[]) =>
    list.filter((raw) => {
      const a = parseAddr(raw).address.toLowerCase();
      if (!a || own.has(a) || seen.has(a)) return false;
      seen.add(a);
      return true;
    });
  const to = pick(sentByMe ? splitAddrs(msg.to) : [msg.sender]);
  if (kind === 'reply') return { to: to.join(', '), cc: '' };
  const restTo = pick(sentByMe ? [] : splitAddrs(msg.to));
  const cc = pick(splitAddrs(msg.cc));
  return { to: [...to, ...restTo].join(', '), cc: cc.join(', ') };
}

/** 有多少外链图片被拦着（清洗时改写到了 data-mail-src）。 */
export function countBlockedImages(html: string): number {
  return (html.match(/data-mail-src=/g) ?? []).length;
}

export type MailPaper = { ink: string; paper: string; link: string; muted: string; font: string };

/**
 * HTML 正文的 iframe 文档：CSP 默认什么都不许连，图片要用户点「显示图片」才放行；
 * iframe 本身还是 sandbox（不给脚本），这是第二、三层防护（第一层在服务端清洗）。
 */
export function mailSrcdoc(html: string, showImages: boolean, p: MailPaper): string {
  const img = showImages ? 'img-src data: https: http:' : 'img-src data:';
  const body = showImages ? html.replace(/data-mail-src=/g, 'src=') : html;
  const csp = `default-src 'none'; style-src 'unsafe-inline'; ${img}; font-src data:`;
  return `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="${csp}"><base target="_blank"><style>html,body{margin:0}body{padding:16px 20px;background:${p.paper};color:${p.ink};font:14px/1.75 ${p.font};word-break:break-word;overflow-wrap:anywhere}img{max-width:100%;height:auto}table{max-width:100%}a{color:${p.link}}blockquote{margin:0 0 0 4px;padding-left:12px;border-left:3px solid ${p.muted};color:${p.muted}}pre{white-space:pre-wrap}</style></head><body>${body}</body></html>`;
}

/** 附件类型色块的字。 */
export function fileBadge(filename: string, contentType = ''): string {
  const ext = (filename.split('.').pop() ?? '').toLowerCase();
  if (['doc', 'docx', 'rtf', 'odt', 'wps'].includes(ext)) return 'DOC';
  if (['xls', 'xlsx', 'csv', 'et', 'ods'].includes(ext)) return 'XLS';
  if (['ppt', 'pptx', 'key', 'dps', 'odp'].includes(ext)) return 'PPT';
  if (ext === 'pdf') return 'PDF';
  if (['zip', 'rar', '7z', 'gz', 'tar'].includes(ext)) return 'ZIP';
  if (ext === 'eml' || contentType === 'message/rfc822') return 'EML';
  if (contentType.startsWith('image/') || ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'svg'].includes(ext))
    return 'IMG';
  if (['txt', 'md', 'log'].includes(ext) || contentType.startsWith('text/')) return 'TXT';
  return 'FILE';
}

export type AttachmentPreviewKind = 'image' | 'pdf' | 'html' | 'text' | 'none';

/** 附件能不能在页面里直接看、用什么看（类型常是 octet-stream，按扩展名兜底）。 */
export function previewKind(filename: string, contentType = ''): AttachmentPreviewKind {
  const ext = (filename.split('.').pop() ?? '').toLowerCase();
  const ct = contentType.toLowerCase();
  if (ct.startsWith('image/') || ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'svg'].includes(ext)) return 'image';
  if (ct === 'application/pdf' || ext === 'pdf') return 'pdf';
  if (ct === 'text/html' || ext === 'html' || ext === 'htm') return 'html';
  if (ct.startsWith('text/') || ['txt', 'md', 'log', 'csv', 'json', 'xml', 'yaml', 'yml', 'ics'].includes(ext))
    return 'text';
  return 'none';
}

/** HTML 附件的 srcdoc：不给任何外链和脚本（iframe 另用空 sandbox）；附件里自带的 data: 图片照常显示。 */
export function attachmentSrcdoc(html: string): string {
  const csp = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:";
  return `<!doctype html><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="${csp}">${html}`;
}

export function fileSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

/** 首次同步范围 → sync_since（RFC3339）；'all' = 空。 */
export function syncSinceFor(range: '7d' | '30d' | '1y' | 'all', now: Date): string {
  const days = { '7d': 7, '30d': 30, '1y': 365, all: 0 }[range];
  if (!days) return '';
  return new Date(now.getTime() - days * 86_400_000).toISOString();
}
