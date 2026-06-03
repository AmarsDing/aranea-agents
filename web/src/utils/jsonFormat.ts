/** Pretty-print JSON text; returns original on parse failure. */
export function formatJsonText(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed) return '';
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2);
  } catch {
    return raw;
  }
}

/** Escape HTML then wrap JSON tokens for syntax-highlight display. */
export function highlightJsonHtml(formatted: string): string {
  const escaped = formatted.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

  return escaped.replace(
    /("(\\u[a-fA-F0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+\-]?\d+)?)/g,
    (match) => {
      let cls = 'json-code-viewer__number';
      if (/^"/.test(match)) {
        cls = /:$/.test(match) ? 'json-code-viewer__key' : 'json-code-viewer__string';
      } else if (/true|false/.test(match)) {
        cls = 'json-code-viewer__boolean';
      } else if (/null/.test(match)) {
        cls = 'json-code-viewer__null';
      }
      return `<span class="${cls}">${match}</span>`;
    },
  );
}
