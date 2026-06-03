/** True when text is a complete JSON object (not a pretty-print line fragment). */
export function isCatalogJsonBlock(text: string): boolean {
  const t = text.trim();
  if (!t.startsWith('{') || !t.endsWith('}')) return false;
  try {
    JSON.parse(t);
    return true;
  } catch {
    return false;
  }
}

/** Filter API blocks; detect legacy line-mode responses. */
export function normalizeCatalogSearchBlocks(raw: string[]): {
  blocks: string[];
  legacyLineMode: boolean;
} {
  if (!raw.length) return { blocks: [], legacyLineMode: false };
  const valid = raw.filter(isCatalogJsonBlock);
  if (valid.length === 0) {
    return { blocks: [], legacyLineMode: true };
  }
  return { blocks: valid, legacyLineMode: false };
}
