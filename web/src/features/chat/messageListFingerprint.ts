import type { Message } from './types';

/** Cheap signature to skip ReAct tool-index rebuilds when only streaming text changes. */
export function messageListStructureFingerprint(messages: Message[]): string {
  if (!messages.length) return '';
  const parts: string[] = [`n:${messages.length}`];
  for (let i = 0; i < messages.length; i++) {
    const m = messages[i]!;
    const contentLen = (m.content_markdown ?? '').length;
    parts.push(`${m.id}|${m.role}|${m.status}|${contentLen}|${(m.options_json ?? '').length}`);
  }
  return parts.join(';');
}
