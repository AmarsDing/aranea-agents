import DOMPurify from "dompurify";
import MarkdownIt from "markdown-it";

const markdown = new MarkdownIt({
  breaks: true,
  html: false,
  linkify: true,
});
markdown.enable(["table", "strikethrough"]);

markdown.renderer.rules.fence = (tokens, idx) => {
  const token = tokens[idx]!;
  const info = (token.info || "").trim();
  const lang = info ? info.split(/\s+/)[0]! : "";
  const langLabel = lang || "code";
  const safeCode = markdown.utils.escapeHtml(token.content);
  const codeClass = lang ? ` class="language-${markdown.utils.escapeHtml(lang)}"` : "";
  return `<div class="code-block">
    <div class="code-block__header">
      <span class="code-block__lang">${markdown.utils.escapeHtml(langLabel)}</span>
      <button type="button" class="code-block__copy" aria-label="复制代码">
        <span class="code-block__copy-icon" aria-hidden="true"></span>
        <span class="code-block__copy-text">复制</span>
      </button>
    </div>
    <pre><code${codeClass}>${safeCode}</code></pre>
  </div>`;
};

export function formatMessageStamp(iso: string): string {
  if (!iso) return "";
  try {
    const d = new Date(iso);
    const now = new Date();
    const sameDay = d.toDateString() === now.toDateString();
    if (sameDay) {
      return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
    }
    const diffDays = Math.floor((now.getTime() - d.getTime()) / 86_400_000);
    if (diffDays < 7) {
      return d.toLocaleString(undefined, {
        weekday: "short",
        hour: "2-digit",
        minute: "2-digit",
      });
    }
    return d.toLocaleString(undefined, {
      month: "short",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return iso;
  }
}

export function renderChatMarkdown(content: string): string {
  return DOMPurify.sanitize(markdown.render(content || ""), {
    ADD_TAGS: ["button"],
    ADD_ATTR: ["type", "aria-label", "aria-hidden"],
  });
}

function closeOpenFences(src: string): string {
  let count = 0;
  for (const line of src.split("\n")) {
    if (/^\s*```/.test(line)) count++;
  }
  if (count % 2 !== 0) return `${src}\n\`\`\``;
  return src;
}

export function renderStreamingChatMarkdown(content: string): string {
  const patched = closeOpenFences(content || "");
  return DOMPurify.sanitize(markdown.render(patched), {
    ADD_TAGS: ["button"],
    ADD_ATTR: ["type", "aria-label", "aria-hidden"],
  });
}
