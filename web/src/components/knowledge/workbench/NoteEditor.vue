<template>
  <div ref="host" class="kb-note-editor" />
</template>

<script setup lang="ts">
// NoteEditor（SP2 §SP2-5/§SP2-6）：CodeMirror 6 + Live Preview 行级装饰 + wikilink 芯片/补全。
// 源文本即真相源：updateListener 同步写回（不防抖——保存正确性优先，消费方如大纲自行防抖）。
import { onBeforeUnmount, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { markdown } from '@codemirror/lang-markdown';
import { EditorState, RangeSetBuilder, type Extension } from '@codemirror/state';
import {
  Decoration,
  EditorView,
  ViewPlugin,
  WidgetType,
  keymap,
  type DecorationSet,
  type ViewUpdate,
} from '@codemirror/view';
import { HighlightStyle, syntaxHighlighting, syntaxTree } from '@codemirror/language';
import { tags } from '@lezer/highlight';
import { autocompletion, closeBrackets } from '@codemirror/autocomplete';
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands';
import {
  parseWikiLinks,
  resolveWikiTarget,
  wikiLinkCompletionSource,
  wikiLinkLabel,
} from '../../../features/knowledge/wikilink';

const props = withDefaults(
  defineProps<{
    content: string;
    /** 只读预览（全文装饰，无光标行回退） */
    readOnly?: boolean;
    /** wikilink 候选（当前库文档 relPath/文件名） */
    candidates?: string[];
    /** P2-5：`[[target#` 标题补全数据源（容器提供：已打开 tab 的大纲标题） */
    getHeadings?: (target: string) => string[];
    /** B4 #8：空查询 [[ 补全的最近引用名次（归一化名 → 名次，0=最近） */
    linkRecencyRank?: ReadonlyMap<string, number>;
  }>(),
  { readOnly: false, candidates: () => [], getHeadings: undefined, linkRecencyRank: undefined },
);

const emit = defineEmits<{
  'update-content': [content: string];
  save: [];
  /** 跳链：目标已存在（target 为链接原文，上层按名归一化解析 docId；heading 供滚动定位） */
  'open-doc': [target: string, heading?: string];
  /** 目标不存在：新建并打开（Obsidian 语义） */
  'create-doc': [target: string];
  /** B4 #8：wikilink 补全落链（target 为选中的原始候选 relPath，供上报 recency） */
  'pick-link': [target: string];
}>();

const { t } = useI18n();
const host = ref<HTMLElement | null>(null);

let view: EditorView | null = null;

// ---------- Live Preview 行级装饰 ----------

/** 光标行集合（readOnly 时为空 → 全文装饰）。 */
function cursorLineSet(v: EditorView): Set<number> {
  const set = new Set<number>();
  if (props.readOnly) return set;
  for (const r of v.state.selection.ranges) {
    const from = v.state.doc.lineAt(r.from).number;
    const to = v.state.doc.lineAt(r.to).number;
    for (let l = from; l <= to; l++) set.add(l);
  }
  return set;
}

/** 代码块（fence/缩进）行区间：syntaxTree 节点行集合。 */
function codeLineSet(v: EditorView): Set<number> {
  const set = new Set<number>();
  syntaxTree(v.state)
    .cursor()
    .iterate((node) => {
      if (node.name !== 'FencedCode' && node.name !== 'CodeBlock') return;
      const from = v.state.doc.lineAt(node.from).number;
      const to = v.state.doc.lineAt(node.to).number;
      for (let l = from; l <= to; l++) set.add(l);
    });
  return set;
}

function onWikiJump(target: string, heading: string) {
  const exists = resolveWikiTarget(target, props.candidates ?? []);
  if (!exists) {
    emit('create-doc', target);
    return;
  }
  // 无 heading 时保持单参数载荷（下游/测试契约：[target]）
  if (heading) emit('open-doc', target, heading);
  else emit('open-doc', target);
}

class WikiLinkWidget extends WidgetType {
  constructor(
    readonly label: string,
    readonly target: string,
    readonly heading: string,
    readonly dangling: boolean,
  ) {
    super();
  }

  eq(other: WikiLinkWidget): boolean {
    return (
      other.label === this.label &&
      other.target === this.target &&
      other.heading === this.heading &&
      other.dangling === this.dangling
    );
  }

  toDOM(): HTMLElement {
    const el = document.createElement('span');
    el.className = 'kb-wikilink' + (this.dangling ? ' kb-wikilink--dangling' : '');
    el.textContent = this.label;
    el.title = this.dangling
      ? t('knowledgePage.workbench.wikilinkDangling')
      : this.heading
        ? `${this.target}#${this.heading}`
        : this.target;
    el.onmousedown = (e: MouseEvent) => {
      // 编辑态 Ctrl/Cmd+点击；预览态单击
      if (!props.readOnly && !e.ctrlKey && !e.metaKey) return;
      e.preventDefault();
      e.stopPropagation();
      onWikiJump(this.target, this.heading);
    };
    return el;
  }

  ignoreEvent(): boolean {
    return false;
  }
}

const INLINE_RE =
  /(\[\[[^\]|#]+?(?:#[^\]|]*?)?(?:\|[^\]]*?)?\]\])|(\*\*|__)(.+?)\2|(\*|_)([^*_]+?)\4|(~~)(.+?)~~|(`)([^`]+?)`/g;

const markDim = Decoration.mark({ class: 'kb-lp-mark' });

type DecoItem = { from: number; to: number; deco: Decoration };

function collectLineDecos(
  v: EditorView,
  line: { from: number; to: number; text: string; number: number },
  codeLines: Set<number>,
  out: DecoItem[],
) {
  const { from, text } = line;

  // line decoration 为 zero-length（行首点装饰）
  if (codeLines.has(line.number)) {
    out.push({ from, to: from, deco: Decoration.line({ class: 'kb-lp-codeblock' }) });
    return;
  }

  // ATX 标题
  const h = /^(#{1,6})\s+/.exec(text);
  if (h) {
    out.push({ from, to: from, deco: Decoration.line({ class: `kb-lp-h${h[1].length}` }) });
    out.push({ from, to: from + h[0].length, deco: markDim });
  }

  // 引用
  if (/^>\s?/.test(text)) {
    out.push({ from, to: from, deco: Decoration.line({ class: 'kb-lp-quote' }) });
  }

  // 列表 marker：无序 → •
  const li = /^(\s*)([-*+])\s+/.exec(text);
  if (li) {
    out.push({
      from: from + li[1].length,
      to: from + li[1].length + 1,
      deco: Decoration.replace({ widget: new BulletWidget() }),
    });
  }

  // 行内样式 + wikilink
  for (const m of text.matchAll(INLINE_RE)) {
    const mFrom = from + (m.index ?? 0);
    const mTo = mFrom + m[0].length;
    if (m[1]) {
      // wikilink → 芯片
      const ref = parseWikiLinks(m[1])[0];
      if (!ref) continue;
      const dangling = !resolveWikiTarget(ref.target, props.candidates ?? []);
      out.push({
        from: mFrom,
        to: mTo,
        deco: Decoration.replace({
          widget: new WikiLinkWidget(wikiLinkLabel(ref), ref.target, ref.heading, dangling),
        }),
      });
    } else if (m[2]) {
      // bold：内容加粗，标记淡化
      out.push({ from: mFrom + 2, to: mTo - 2, deco: Decoration.mark({ class: 'kb-lp-bold' }) });
      out.push({ from: mFrom, to: mFrom + 2, deco: markDim });
      out.push({ from: mTo - 2, to: mTo, deco: markDim });
    } else if (m[4]) {
      out.push({ from: mFrom + 1, to: mTo - 1, deco: Decoration.mark({ class: 'kb-lp-italic' }) });
      out.push({ from: mFrom, to: mFrom + 1, deco: markDim });
      out.push({ from: mTo - 1, to: mTo, deco: markDim });
    } else if (m[6]) {
      out.push({ from: mFrom + 2, to: mTo - 2, deco: Decoration.mark({ class: 'kb-lp-strike' }) });
      out.push({ from: mFrom, to: mFrom + 2, deco: markDim });
      out.push({ from: mTo - 2, to: mTo, deco: markDim });
    } else if (m[8]) {
      out.push({ from: mFrom, to: mTo, deco: Decoration.mark({ class: 'kb-lp-code' }) });
    }
  }
}

class BulletWidget extends WidgetType {
  eq(): boolean {
    return true;
  }
  toDOM(): HTMLElement {
    const el = document.createElement('span');
    el.className = 'kb-lp-bullet';
    el.textContent = '•';
    return el;
  }
}

const livePreview = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet;
    constructor(v: EditorView) {
      this.decorations = this.build(v);
    }
    update(u: ViewUpdate) {
      if (u.docChanged || u.selectionSet || u.viewportChanged) {
        this.decorations = this.build(u.view);
      }
    }
    build(v: EditorView): DecorationSet {
      const items: DecoItem[] = [];
      const cursorLines = cursorLineSet(v);
      const codeLines = codeLineSet(v);
      for (const vr of v.visibleRanges) {
        let line = v.state.doc.lineAt(vr.from);
        for (;;) {
          if (!cursorLines.has(line.number)) {
            collectLineDecos(v, line, codeLines, items);
          }
          if (line.to >= vr.to || line.number >= v.state.doc.lines) break;
          line = v.state.doc.line(line.number + 1);
        }
      }
      // RangeSetBuilder 要求按 from 升序：先排序再一次性 add
      items.sort((a, b) => a.from - b.from || a.to - b.to);
      const builder = new RangeSetBuilder<Decoration>();
      for (const it of items) builder.add(it.from, it.to, it.deco);
      return builder.finish();
    }
  },
  { decorations: (v) => v.decorations },
);

// ---------- 深空主题 ----------

const kbTheme = EditorView.theme(
  {
    '&': {
      backgroundColor: 'transparent',
      color: 'var(--kb-text-primary)',
      fontSize: '14px',
      height: '100%',
    },
    '.cm-scroller': {
      fontFamily: "'JetBrains Mono', 'Fira Code', Consolas, monospace",
      lineHeight: '1.75',
    },
    '.cm-content': { padding: '18px 22px', caretColor: 'var(--kb-accent-cyan)' },
    '.cm-cursor': { borderLeftColor: 'var(--kb-accent-cyan)' },
    '&.cm-focused': { outline: 'none' },
    '.cm-selectionBackground, &.cm-focused .cm-selectionBackground': {
      backgroundColor: 'rgba(79, 216, 255, 0.15)',
    },
    '.cm-tooltip': {
      background: 'var(--kb-bg-deep)',
      border: '1px solid var(--kb-glass-border)',
      color: 'var(--kb-text-primary)',
    },
    '.cm-tooltip-autocomplete ul li[aria-selected]': {
      background: 'rgba(79, 216, 255, 0.15)',
      color: 'var(--kb-text-primary)',
    },
    // Live Preview 样式
    '.kb-lp-mark': { opacity: '0.35' },
    '.kb-lp-bold': { fontWeight: '700' },
    '.kb-lp-italic': { fontStyle: 'italic' },
    '.kb-lp-strike': { textDecoration: 'line-through' },
    '.kb-lp-code': {
      background: 'rgba(79, 216, 255, 0.08)',
      borderRadius: '4px',
      padding: '1px 4px',
    },
    '.kb-lp-codeblock': { background: 'rgba(79, 216, 255, 0.05)' },
    '.kb-lp-quote': {
      borderLeft: '3px solid var(--kb-accent-cyan)',
      paddingLeft: '10px',
      color: 'var(--kb-text-dim)',
    },
    '.kb-lp-h1': { fontSize: '1.6em', fontWeight: '700', color: 'var(--kb-accent-cyan)' },
    '.kb-lp-h2': { fontSize: '1.4em', fontWeight: '700', color: 'var(--kb-accent-cyan)' },
    '.kb-lp-h3': { fontSize: '1.25em', fontWeight: '600' },
    '.kb-lp-h4': { fontSize: '1.15em', fontWeight: '600' },
    '.kb-lp-h5': { fontSize: '1.08em', fontWeight: '600' },
    '.kb-lp-h6': { fontSize: '1.02em', fontWeight: '600', color: 'var(--kb-text-dim)' },
    '.kb-lp-bullet': { color: 'var(--kb-accent-cyan)' },
    // U3 wikilink 芯片（研究 F2）：accent 10% 底 + 6px 圆角；hover 底色升 20% + 微光
    '.kb-wikilink': {
      display: 'inline-block',
      padding: '1px 7px',
      borderRadius: '6px',
      background: 'color-mix(in srgb, var(--kb-accent-cyan) 10%, transparent)',
      border: '1px solid color-mix(in srgb, var(--kb-accent-cyan) 22%, transparent)',
      color: 'var(--kb-accent-cyan)',
      cursor: 'pointer',
      transition: 'background 150ms ease-out, border-color 150ms ease-out, box-shadow 150ms ease-out',
    },
    '.kb-wikilink:hover': {
      background: 'color-mix(in srgb, var(--kb-accent-cyan) 20%, transparent)',
      borderColor: 'color-mix(in srgb, var(--kb-accent-cyan) 38%, transparent)',
      boxShadow: '0 0 12px color-mix(in srgb, var(--kb-accent-cyan) 25%, transparent)',
    },
    // 断链降级：虚线边 + 降透明度，与可解析链拉开视觉层级
    '.kb-wikilink--dangling': {
      color: 'var(--kb-text-faint)',
      border: '1px dashed color-mix(in srgb, var(--kb-text-dim) 45%, transparent)',
      background: 'color-mix(in srgb, var(--kb-text-dim) 8%, transparent)',
      opacity: '0.6',
    },
    '.kb-wikilink--dangling:hover': {
      background: 'color-mix(in srgb, var(--kb-text-dim) 14%, transparent)',
      borderColor: 'color-mix(in srgb, var(--kb-text-dim) 60%, transparent)',
      boxShadow: 'none',
      opacity: '0.8',
    },
  },
  { dark: true },
);

const kbHighlight = HighlightStyle.define([
  { tag: tags.keyword, color: '#9d6bff' },
  { tag: tags.string, color: '#7ee0a3' },
  { tag: tags.comment, color: '#5a6a85', fontStyle: 'italic' },
  { tag: tags.link, color: '#4fd8ff' },
  { tag: tags.monospace, color: '#7ee0a3' },
]);

// ---------- 装配 ----------

function buildExtensions(): Extension[] {
  const exts: Extension[] = [
    history(),
    keymap.of([
      {
        key: 'Mod-s',
        preventDefault: true,
        run: () => {
          emit('save');
          return true;
        },
      },
      ...defaultKeymap,
      ...historyKeymap,
    ]),
    markdown(),
    syntaxHighlighting(kbHighlight),
    closeBrackets(),
    autocompletion({
      override: [
        wikiLinkCompletionSource(
          () => props.candidates ?? [],
          (target) => props.getHeadings?.(target) ?? [],
          () => props.linkRecencyRank ?? new Map(),
          (target) => emit('pick-link', target),
        ),
      ],
      activateOnTyping: true,
      maxRenderedOptions: 20,
    }),
    kbTheme,
    livePreview,
    EditorView.lineWrapping,
    EditorView.updateListener.of((u: ViewUpdate) => {
      if (u.docChanged) emit('update-content', u.state.doc.toString());
    }),
  ];
  if (props.readOnly) {
    exts.push(EditorState.readOnly.of(true), EditorView.editable.of(false));
  }
  return exts;
}

onMounted(() => {
  if (!host.value) return;
  view = new EditorView({
    state: EditorState.create({ doc: props.content, extensions: buildExtensions() }),
    parent: host.value,
  });
});

onBeforeUnmount(() => {
  view?.destroy();
  view = null;
});

/** 供右栏大纲面板滚动定位（SP2-5）：按文档偏移选区 + scrollIntoView。 */
function scrollToOffset(offset: number) {
  if (!view) return;
  const pos = Math.min(Math.max(0, offset), view.state.doc.length);
  view.dispatch({ selection: { anchor: pos }, effects: EditorView.scrollIntoView(pos, { y: 'start' }) });
  view.focus();
}

defineExpose({ scrollToOffset });
</script>

<style lang="sass" scoped>
.kb-note-editor
  flex: 1
  min-height: 0
  height: 100%

  :deep(.cm-editor)
    height: 100%
</style>
