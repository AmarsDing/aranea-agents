import { describe, it, expect, afterEach } from 'vitest';
import { createApp, nextTick, reactive, type App as VueApp } from 'vue';
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

/**
 * Mount ActionBlock with a reactive activity wrapper, so the test can mutate
 * fields (e.g. status) and assert that the DOM updates without a re-mount.
 * Mirrors the upstream data flow: WS events → useActivityTimeline mutates the
 * Activity in-place → ActionBlock re-renders via prop reactivity.
 */
function mountActionReactive(activity: ActionEvent): {
  container: HTMLElement;
  update: (patch: Partial<ActionEvent['tool']>) => void;
} {
  const reactiveActivity = reactive(activity);
  const container = document.createElement('div');
  document.body.appendChild(container);
  const app: VueApp = createApp(ActionBlock, { activity: reactiveActivity });
  app.use(i18n);
  app.mount(container);
  return {
    container,
    update(patch) {
      Object.assign(reactiveActivity.tool, patch);
    },
  };
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

// Chat UI fix: statusClass must cover ALL status values including 'cancelled'.
// Previously 'cancelled' had an icon (⊘) but no CSS modifier class, so
// cancelled tools rendered with no status color (muted) — inconsistent
// with the design spec which requires a muted/cancelled visual state.
describe('ActionBlock statusClass', () => {
  const classCases: Array<[ActionEvent['tool']['status'], string]> = [
    ['running', 'act-activity__status--running'],
    ['success', 'act-activity__status--success'],
    ['failed', 'act-activity__status--failed'],
    ['blocked', 'act-activity__status--blocked'],
    ['cancelled', 'act-activity__status--cancelled'],
  ];

  for (const [status, wantClass] of classCases) {
    it(`applies ${wantClass} for status=${status}`, () => {
      const c = mountAction(makeAction({ status }));
      const statusSpan = c.querySelector('.act-activity__status');
      expect(statusSpan?.className).toContain(wantClass);
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

/**
 * Chat UI #1: Tool name + parameters are too noisy when expanded by default.
 * Users should see the compact header (icon + label + status + duration);
 * the full detail block (tool name, args, result, error) must start collapsed
 * and be revealed on click. This matches the user's screenshot request.
 */
describe('ActionBlock default collapse state', () => {
  it('hides the detail block by default (tool name + params not visible)', () => {
    const c = mountAction(
      makeAction({
        toolName: 'shell',
        toolCategory: 'shell',
        arguments: JSON.stringify({ command: 'ls -la /tmp' }),
        result: 'file1\nfile2',
      }),
    );
    // The compact header is always visible.
    expect(c.querySelector('.act-activity__header')).toBeTruthy();
    // The detail block (where tool name + args/result are rendered) must NOT
    // be in the DOM on first render — it appears only after the user clicks
    // the header to expand.
    expect(c.querySelector('.act-activity__detail')).toBeNull();
    // Sanity: GenericToolDetail (which renders toolName / arguments / result)
    // is the mapped detail component for category=shell-untouched tools, but
    // for shell category the per-category ShellToolDetail renders instead.
    // Either way, the <component :is="detailComponent"> must not be mounted
    // when collapsed. The absence of .act-activity__detail is the contract.
  });

  it('reveals the detail block after the user clicks the header', async () => {
    const c = mountAction(
      makeAction({
        toolName: 'shell',
        toolCategory: 'shell',
        arguments: JSON.stringify({ command: 'ls' }),
      }),
    );
    expect(c.querySelector('.act-activity__detail')).toBeNull();
    const header = c.querySelector('.act-activity__header') as HTMLElement;
    header.click();
    await nextTick();
    expect(c.querySelector('.act-activity__detail')).toBeTruthy();
  });
});

/**
 * Chat UI #2: Tool status should update in real-time as the backend emits
 * `updated` / `completed` / `failed` events. The data flow is:
 *   WS event → useActivityTimeline mutates the Activity in the in-memory map
 *   → activityTree computed re-runs → renderItems in ActivityStream re-runs
 *   → ActionBlock receives a new `activity` prop → statusIcon re-evaluates.
 * A page refresh loads the same data from the API and renders correctly, so
 * the bug is purely a reactivity gap between the in-memory state and the
 * rendered DOM. This test pins the contract: status transitions must be
 * reflected in the DOM without re-mounting the component.
 */
describe('ActionBlock status reactivity', () => {
  it('updates the status icon when activity.tool.status changes (no refresh)', async () => {
    const { container, update } = mountActionReactive(
      makeAction({ toolName: 'shell', toolCategory: 'shell', status: 'running' }),
    );
    expect(container.querySelector('.act-activity__status')?.textContent).toBe('⏳');

    // Simulate the backend's `completed` event arriving via WS.
    update({ status: 'success' });
    await nextTick();
    expect(container.querySelector('.act-activity__status')?.textContent).toBe('✓');

    // Simulate a later `failed` event on a different tool run.
    update({ status: 'failed' });
    await nextTick();
    expect(container.querySelector('.act-activity__status')?.textContent).toBe('✗');
  });

  it('updates the status CSS class in lockstep with the status icon', async () => {
    const { container, update } = mountActionReactive(
      makeAction({ toolName: 'shell', toolCategory: 'shell', status: 'running' }),
    );
    const statusEl = () => container.querySelector('.act-activity__status') as HTMLElement;
    expect(statusEl().classList.contains('act-activity__status--running')).toBe(true);

    update({ status: 'success' });
    await nextTick();
    expect(statusEl().classList.contains('act-activity__status--success')).toBe(true);
    expect(statusEl().classList.contains('act-activity__status--running')).toBe(false);
  });
});
