import { afterEach, describe, expect, it } from 'vitest';
import { reviewShortcut } from '../reviewShortcuts';
afterEach(() => {
  document.body.innerHTML = '';
});

describe('review shortcuts never submit while editing', () => {
  it.each(['input', 'textarea', 'select', 'button', 'a'])('ignores letters on %s controls', (tag) => {
    const control = document.createElement(tag);
    document.body.append(control);
    let result: ReturnType<typeof reviewShortcut> = 'approve';
    control.addEventListener('keydown', (event) => {
      result = reviewShortcut(event as KeyboardEvent);
    });
    control.dispatchEvent(new KeyboardEvent('keydown', { key: 'a' }));
    expect(result).toBeNull();
  });
  it('ignores IME, repeat and modified key sequences', () => {
    for (const init of [
      { isComposing: true },
      { repeat: true },
      { ctrlKey: true },
      { altKey: true },
      { metaKey: true },
    ]) {
      expect(reviewShortcut(new KeyboardEvent('keydown', { key: 'a', ...init }))).toBeNull();
    }
  });
  it('does not act behind a visible modal or dropdown', () => {
    document.body.innerHTML = '<div class="q-dialog"></div>';
    expect(reviewShortcut(new KeyboardEvent('keydown', { key: 'a' }))).toBeNull();
    document.body.innerHTML = '<div class="q-menu"></div>';
    expect(reviewShortcut(new KeyboardEvent('keydown', { key: 'r' }))).toBeNull();
  });
  it('recognizes deliberate shortcuts outside editable content', () => {
    expect(reviewShortcut(new KeyboardEvent('keydown', { key: 'A' }))).toBe('approve');
    expect(reviewShortcut(new KeyboardEvent('keydown', { key: 'n' }))).toBe('next');
  });
});
