export function reviewShortcut(event: KeyboardEvent): 'approve' | 'reject' | 'next' | null {
  if (
    event.defaultPrevented ||
    event.repeat ||
    event.isComposing ||
    event.keyCode === 229 ||
    event.ctrlKey ||
    event.altKey ||
    event.metaKey
  )
    return null;
  const target = event.target;
  if (
    target instanceof Element &&
    target.closest(
      'input, textarea, select, button, a, [contenteditable]:not([contenteditable="false"]), [role="dialog"], [role="combobox"], [role="textbox"]',
    )
  )
    return null;
  // Never let a page-level shortcut activate behind a modal or dropdown.
  if (typeof document !== 'undefined' && document.querySelector('.q-dialog, .q-menu')) return null;
  return ({ a: 'approve', r: 'reject', n: 'next' } as const)[event.key.toLowerCase() as 'a' | 'r' | 'n'] ?? null;
}
