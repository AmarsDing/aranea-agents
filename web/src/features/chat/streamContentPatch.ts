import type { Message } from './types';

type StreamExtras = {
  reasoning_markdown?: string;
  reasoning_content?: string;
  reasoning_as_display?: boolean;
};

function reasoningFromExtras(extras: StreamExtras): string {
  return (extras.reasoning_markdown ?? extras.reasoning_content ?? '').trim();
}

export function parseMessageExtras(optionsJson: string): StreamExtras {
  try {
    return JSON.parse(optionsJson || '{}') as StreamExtras;
  } catch {
    return {};
  }
}

export function mergeMessageExtras(optionsJson: string, patch: StreamExtras): string {
  return JSON.stringify({ ...parseMessageExtras(optionsJson), ...patch });
}

/** Patch streaming assistant row: accumulate text and/or reasoning separately from final answer. */
export function patchStreamingMessage(
  messages: Message[],
  messageId: string,
  patch: { text?: string; reasoning?: string; replaceText?: string; replaceReasoning?: string; status?: string },
): Message[] {
  return messages.map((m) => {
    if (m.id !== messageId) return m;
    let content = m.content_markdown;
    if (patch.replaceText !== undefined) {
      content = patch.replaceText;
    } else if (patch.text) {
      content = `${content}${patch.text}`;
    }
    const extras = parseMessageExtras(m.options_json);
    let reasoning = reasoningFromExtras(extras);
    if (patch.replaceReasoning !== undefined) {
      reasoning = patch.replaceReasoning;
    } else if (patch.reasoning) {
      reasoning = `${reasoning}${patch.reasoning}`;
    }
    return {
      ...m,
      content_markdown: content,
      status: patch.status ?? m.status,
      options_json: mergeMessageExtras(m.options_json, {
        reasoning_markdown: reasoning.trim() ? reasoning : undefined,
      }),
      reasoning_markdown: reasoning.trim() || undefined,
    };
  });
}

export function reasoningMarkdown(message: Message): string {
  if (message.reasoning_markdown?.trim()) return message.reasoning_markdown.trim();
  return reasoningFromExtras(parseMessageExtras(message.options_json));
}

/**
 * Check if content_markdown is a reasoning fallback (LLM produced only reasoning, no separate reply).
 *
 * @deprecated Activity-First architecture resolves this at the backend via
 * ActivityProjector.OnReasoningDone(reasoningAsDisplay=true) → kind=reply.
 * This function is only needed for the legacy message-inference fallback path.
 */
export function isReasoningAsDisplay(message: Message): boolean {
  const extras = parseMessageExtras(message.options_json);
  return extras.reasoning_as_display === true;
}

/** Map legacy reasoning_content from server rows to reasoning_markdown for UI replay. */
export function normalizeServerMessageOptions(optionsJson: string): string {
  const extras = parseMessageExtras(optionsJson);
  if (!reasoningFromExtras(extras) && extras.reasoning_content?.trim()) {
    return mergeMessageExtras(optionsJson, { reasoning_markdown: extras.reasoning_content.trim() });
  }
  if (extras.reasoning_markdown?.trim() && !extras.reasoning_content?.trim()) {
    return mergeMessageExtras(optionsJson, { reasoning_content: extras.reasoning_markdown.trim() });
  }
  return optionsJson;
}
