// wikilink 纯函数单测（SP2 §SP2-6）：解析 / 别名 / 归一化 / 存在性 / 补全 source。
import { describe, expect, it } from 'vitest';
import { CompletionContext } from '@codemirror/autocomplete';
import { EditorState } from '@codemirror/state';
import {
  normalizeTargetName,
  parseWikiLinks,
  resolveWikiTarget,
  wikiLinkCompletionSource,
  wikiLinkLabel,
} from '../wikilink';

describe('parseWikiLinks', () => {
  it('parses plain / alias / heading forms with offsets', () => {
    const doc = '见 [[Note]] 与 [[dir/Other.md#第二节|别名]] 结尾';
    const refs = parseWikiLinks(doc);
    expect(refs).toHaveLength(2);
    expect(refs[0]).toMatchObject({ target: 'Note', alias: '', heading: '' });
    expect(doc.slice(refs[0].from, refs[0].to)).toBe('[[Note]]');
    expect(refs[1]).toMatchObject({ target: 'dir/Other.md', heading: '第二节', alias: '别名' });
    expect(doc.slice(refs[1].from, refs[1].to)).toBe('[[dir/Other.md#第二节|别名]]');
  });

  it('skips empty targets', () => {
    expect(parseWikiLinks('[[ ]]')).toHaveLength(0);
    expect(parseWikiLinks('no links here')).toHaveLength(0);
  });
});

describe('wikiLinkLabel / normalizeTargetName / resolveWikiTarget', () => {
  it('label prefers alias', () => {
    expect(wikiLinkLabel({ target: 'Note', alias: '别名' })).toBe('别名');
    expect(wikiLinkLabel({ target: 'Note', alias: '' })).toBe('Note');
  });

  it('normalizes dir prefix and extension case-insensitively', () => {
    expect(normalizeTargetName('dir/Sub/My Note.MD')).toBe('my note');
    expect(normalizeTargetName('plain')).toBe('plain');
    expect(normalizeTargetName('a/b/c.markdown')).toBe('c');
  });

  it('resolves target existence against relPaths or names', () => {
    const candidates = ['docs/Alpha.md', 'Beta.markdown', 'gamma.txt'];
    expect(resolveWikiTarget('Alpha', candidates)).toBe(true);
    expect(resolveWikiTarget('docs/alpha.md', candidates)).toBe(true);
    expect(resolveWikiTarget('beta', candidates)).toBe(true);
    expect(resolveWikiTarget('Missing', candidates)).toBe(false);
    expect(resolveWikiTarget('', candidates)).toBe(false);
  });
});

describe('wikiLinkCompletionSource', () => {
  function ctxOf(doc: string, pos?: number): CompletionContext {
    const state = EditorState.create({ doc });
    return new CompletionContext(state, pos ?? doc.length, false);
  }

  const source = wikiLinkCompletionSource(() => ['docs/Alpha.md', 'Beta.md', 'dir/Gamma.md']);

  it('offers candidates after [[ prefix', () => {
    const res = source(ctxOf('start [['));
    expect(res).not.toBeNull();
    expect(res?.from).toBe('start [['.length);
    expect(res?.options.map((o) => o.label)).toEqual(['alpha', 'beta', 'gamma']);
  });

  it('filters by typed query and preserves alias/heading suffix validity', () => {
    const res = source(ctxOf('x [[al'));
    expect(res?.options.map((o) => o.label)).toEqual(['alpha']);
    const validFor = res?.validFor as RegExp;
    expect(validFor.test('pha')).toBe(true);
    expect(validFor.test('a|b')).toBe(false);
  });

  it('returns null without [[ prefix or when no candidates match', () => {
    expect(source(ctxOf('plain text'))).toBeNull();
    expect(source(ctxOf('x [[zzz'))).toBeNull();
  });
});

describe('wikiLinkCompletionSource #heading（P2-5）', () => {
  function ctxOf(doc: string, pos?: number): CompletionContext {
    const state = EditorState.create({ doc });
    return new CompletionContext(state, pos ?? doc.length, false);
  }

  const headings: Record<string, string[]> = {
    beta: ['快速开始', 'Second Section', '附录'],
  };
  const source = wikiLinkCompletionSource(
    () => ['docs/Alpha.md', 'Beta.md'],
    (target) => headings[target.trim().toLowerCase()] ?? [],
  );

  it('offers all headings right after [[target#', () => {
    const res = source(ctxOf('x [[Beta#'));
    expect(res).not.toBeNull();
    expect(res?.from).toBe('x [[Beta#'.length); // 插入点在 # 之后
    expect(res?.options.map((o) => o.label)).toEqual(['快速开始', 'Second Section', '附录']);
  });

  it('filters headings by typed partial (case-insensitive)', () => {
    const res = source(ctxOf('x [[Beta#sec'));
    expect(res?.options.map((o) => o.label)).toEqual(['Second Section']);
    expect(res?.from).toBe('x [[Beta#'.length);
  });

  it('returns null when doc has no headings or no getHeadings provided', () => {
    expect(source(ctxOf('x [[Alpha#'))).toBeNull();
    const noHeadings = wikiLinkCompletionSource(() => ['Beta.md']);
    expect(noHeadings(ctxOf('x [[Beta#'))).toBeNull();
  });

  it('doc-name completion still works when no # typed', () => {
    const res = source(ctxOf('x [[al'));
    expect(res?.options.map((o) => o.label)).toEqual(['alpha']);
  });
});

describe('wikiLinkCompletionSource 最近引用排序（B4 #8）', () => {
  function ctxOf(doc: string, pos?: number): CompletionContext {
    const state = EditorState.create({ doc });
    return new CompletionContext(state, pos ?? doc.length, false);
  }

  // gamma 最近一次引用，alpha 其次；beta 无记录。
  const recency = new Map([
    ['gamma', 0],
    ['alpha', 1],
  ]);
  const source = wikiLinkCompletionSource(
    () => ['docs/Alpha.md', 'Beta.md', 'dir/Gamma.md'],
    undefined,
    () => recency,
  );

  it('empty query orders by recency rank, unranked keep original order', () => {
    const res = source(ctxOf('start [['));
    expect(res?.options.map((o) => o.label)).toEqual(['gamma', 'alpha', 'beta']);
  });

  it('non-empty query ignores recency (document order preserved)', () => {
    const res = source(ctxOf('x [[a')); // alpha/beta/gamma 均含 a
    expect(res?.options.map((o) => o.label)).toEqual(['alpha', 'beta', 'gamma']);
  });

  it('no recency provider keeps original order', () => {
    const plain = wikiLinkCompletionSource(() => ['docs/Alpha.md', 'dir/Gamma.md']);
    const res = plain(ctxOf('start [['));
    expect(res?.options.map((o) => o.label)).toEqual(['alpha', 'gamma']);
  });

  it('onPick fires with raw candidate when an option is applied', () => {
    const picks: string[] = [];
    const src = wikiLinkCompletionSource(
      () => ['docs/Alpha.md'],
      undefined,
      undefined,
      (target) => picks.push(target),
    );
    const doc = 'x [[';
    const res = src(ctxOf(doc));
    expect(res).not.toBeNull();
    const opt = res!.options[0];
    const state = EditorState.create({ doc });
    const dispatched: unknown[] = [];
    const fakeView = { state, dispatch: (tr: unknown) => dispatched.push(tr) };
    (opt.apply as (v: unknown, c: unknown, from: number, to: number) => void)(fakeView, opt, res!.from, doc.length);
    expect(picks).toEqual(['docs/Alpha.md']);
    expect(dispatched).toHaveLength(1); // 标准插入事务仍派发
  });
});
