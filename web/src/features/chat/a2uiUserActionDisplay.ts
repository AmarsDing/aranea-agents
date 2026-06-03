/** Parse and summarize A2UI userAction lines in user_message content. */

import type { A2UIUserActionPayload } from './a2uiUserAction';

export function parseUserActionFromContent(content: string): A2UIUserActionPayload | null {
  const raw = (content || '').trim();
  if (!raw.startsWith('{')) return null;
  try {
    const obj = JSON.parse(raw) as { userAction?: A2UIUserActionPayload };
    const ua = obj?.userAction;
    if (!ua || typeof ua !== 'object' || !ua.name) return null;
    return ua;
  } catch {
    return null;
  }
}

export function formatUserActionUserMarkdown(payload: A2UIUserActionPayload): string {
  const ctxKeys = Object.keys(payload.context ?? {});
  const ctxHint = ctxKeys.length ? ` · ${ctxKeys.join(', ')}` : '';
  return `**A2UI 操作** · \`${payload.name}\`${ctxHint}`;
}
