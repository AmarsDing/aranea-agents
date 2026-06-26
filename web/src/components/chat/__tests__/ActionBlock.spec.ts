import { describe, it, expect, afterEach } from 'vitest';
import { createApp, type App as VueApp } from 'vue';
import { createI18n } from 'vue-i18n';
import ActionBlock from '../ActionBlock.vue';
import type { ActionEvent } from '../../../features/chat/streamEventTypes';

const i18n = createI18n({
  legacy: false,
  locale: 'en',
  messages: { en: {} },
});

/** Build a minimal ActionEvent with sensible defaults. */
function makeAction(overrides: Partial<ActionEvent['tool']> & { id?: string } = {}): ActionEvent {
  return {
    id: overrides.id ?? 'act-1',
    kind: 'action',
    tool: {
      toolName: 'shell',
      toolLabel: 'Shell',
      toolCategory: 'shell',
      status: 'success',
      durationMs: 100,
      arguments: null,
      result: null,
      error: null,
      ...overrides,
    },
  };
}

function mountAction(activity: ActionEvent): HTMLElement {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const app: VueApp = createApp(ActionBlock, { activity });
  app.use(i18n);
  app.mount(container);
  return container;
}

afterEach(() => {
  document.body.innerHTML = '';
});

describe('ActionBlock toolIcon', () => {
  const iconCases: Array<[string | undefined, string]> = [
    ['shell', '$'],
    ['browser', '🌐'],
    ['file_read', '📖'],
    ['file_write', '✏️'],
    ['file_search', '🔍'],
    ['web_search', '🔎'],
    ['mcp', '🔌'],
    ['code', '💻'],
    ['todo', '✅'],
    ['other', '🔧'],
    [undefined, '🔧'],
    ['unknown_category', '🔧'],
  ];

  for (const [cat, wantIcon] of iconCases) {
    it(`renders ${wantIcon} for category=${cat}`, () => {
      const c = mountAction(makeAction({ toolCategory: cat }));
      expect(c.querySelector('.act-activity__icon')?.textContent).toBe(wantIcon);
    });
  }
});

describe('ActionBlock headerHint', () => {
  it('extracts command from shell tool args', () => {
    const c = mountAction(
      makeAction({
        toolCategory: 'shell',
        arguments: JSON.stringify({ command: 'ls -la /tmp' }),
      }),
    );
    expect(c.querySelector('.act-activity__hint')?.textContent).toBe('ls -la /tmp');
  });

  it('extracts cmd from shell tool args (fallback key)', () => {
    const c = mountAction(
      makeAction({
        toolCategory: 'shell',
        arguments: JSON.stringify({ cmd: 'echo hi' }),
      }),
    );
    expect(c.querySelector('.act-activity__hint')?.textContent).toBe('echo hi');
  });

  it('extracts url from browser tool args', () => {
    const c = mountAction(
      makeAction({
        toolCategory: 'browser',
        arguments: JSON.stringify({ url: 'https://example.com/path' }),
      }),
    );
    expect(c.querySelector('.act-activity__hint')?.textContent).toBe('https://example.com/path');
  });

  it('extracts path from file_read tool args', () => {
    const c = mountAction(
      makeAction({
        toolCategory: 'file_read',
        arguments: JSON.stringify({ path: '/app/src/main.ts' }),
      }),
    );
    expect(c.querySelector('.act-activity__hint')?.textContent).toBe('/app/src/main.ts');
  });

  it('extracts file_path from file_write tool args (fallback key)', () => {
    const c = mountAction(
      makeAction({
        toolCategory: 'file_write',
        arguments: JSON.stringify({ file_path: '/app/config.yml' }),
      }),
    );
    expect(c.querySelector('.act-activity__hint')?.textContent).toBe('/app/config.yml');
  });

  it('extracts query from web_search tool args', () => {
    const c = mountAction(
      makeAction({
        toolCategory: 'web_search',
        arguments: JSON.stringify({ query: 'golang testing best practices' }),
      }),
    );
    expect(c.querySelector('.act-activity__hint')?.textContent).toBe('golang testing best practices');
  });

  it('extracts method from mcp tool args', () => {
    const c = mountAction(
      makeAction({
        toolCategory: 'mcp',
        arguments: JSON.stringify({ method: 'tools/list' }),
      }),
    );
    expect(c.querySelector('.act-activity__hint')?.textContent).toBe('tools/list');
  });

  it('truncates long shell commands', () => {
    const longCmd = 'x'.repeat(100);
    const c = mountAction(
      makeAction({
        toolCategory: 'shell',
        arguments: JSON.stringify({ command: longCmd }),
      }),
    );
    const hint = c.querySelector('.act-activity__hint')?.textContent ?? '';
    expect(hint.length).toBeLessThanOrEqual(81); // 80 + ellipsis
    expect(hint.endsWith('…')).toBe(true);
  });

  it('returns empty hint when args are null', () => {
    const c = mountAction(makeAction({ toolCategory: 'shell', arguments: null }));
    const hint = c.querySelector('.act-activity__hint');
    // No hint element rendered when empty (v-if)
    expect(hint).toBeNull();
  });
});

describe('ActionBlock statusIcon', () => {
  const statusCases: Array<[ActionEvent['tool']['status'], string]> = [
    ['running', '⏳'],
    ['success', '✓'],
    ['failed', '✗'],
    ['blocked', '🔒'],
    ['cancelled', '⊘'],
  ];

  for (const [status, wantIcon] of statusCases) {
    it(`renders ${wantIcon} for status=${status}`, () => {
      const c = mountAction(makeAction({ status }));
      expect(c.querySelector('.act-activity__status')?.textContent).toBe(wantIcon);
    });
  }
});

describe('ActionBlock todo interception', () => {
  it('renders TodoInlineList instead of normal header for todo_write', () => {
    const c = mountAction(
      makeAction({
        toolName: 'todo_write',
        toolLabel: 'Todo Write',
        toolCategory: 'todo',
        arguments: JSON.stringify({ todos: [{ content: 'task', status: 'pending' }] }),
      }),
    );
    // TodoInlineList is rendered; normal act-activity__header is NOT rendered
    expect(c.querySelector('.act-activity__header')).toBeNull();
  });

  it('renders normal header for non-todo tools', () => {
    const c = mountAction(makeAction({ toolName: 'shell', toolCategory: 'shell' }));
    expect(c.querySelector('.act-activity__header')).toBeTruthy();
  });
});

describe('ActionBlock duration', () => {
  it('renders duration when present', () => {
    const c = mountAction(makeAction({ durationMs: 1500 }));
    expect(c.querySelector('.act-activity__duration')?.textContent).toBeTruthy();
  });

  it('does not render duration when null', () => {
    const c = mountAction(makeAction({ durationMs: null }));
    expect(c.querySelector('.act-activity__duration')).toBeNull();
  });
});
