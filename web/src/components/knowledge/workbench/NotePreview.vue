<template>
  <div ref="root" class="kb-note-preview" @click="onLinkClick">
    <!-- eslint-disable-next-line vue/no-v-html -- 经 markdown-it(html:false) + DOMPurify 双重消毒 -->
    <div class="kb-note-preview__content" v-html="renderedHtml" />
  </div>
</template>

<script setup lang="ts">
// NotePreview（G-04）：真 Markdown 渲染预览层，替代「CodeMirror readOnly 行级装饰」假预览。
// 管线：fence/行内代码保护下的 wikilink/embed 预处理（转标准链接，协议标记状态）
//   → markdown-it 渲染（表格/删除线/图片/代码块）→ DOMPurify 消毒（白名单协议）
//   → 任务列表复选框注入（自有安全标记，消毒之后注入）。
// wikilink 语义复用 wikilink.ts：点击 = open-doc（可解析），dangling = create-doc（Obsidian 语义）。
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import DOMPurify from 'dompurify';
import MarkdownIt from 'markdown-it';
import { parseOutline } from '../../../features/knowledge/outline';
import { resolveWikiTarget } from '../../../features/knowledge/wikilink';

const props = withDefaults(
  defineProps<{
    content: string;
    /** wikilink 存在性判定候选（当前库文档 relPath） */
    candidates?: string[];
  }>(),
  { candidates: () => [] },
);

const emit = defineEmits<{
  /** 跳链：目标已存在 */
  'open-doc': [target: string, heading?: string];
  /** 目标不存在：新建并打开 */
  'create-doc': [target: string];
}>();

const { t } = useI18n();
const root = ref<HTMLElement | null>(null);

// ---------- wikilink / embed 预处理 ----------

const WIKILINK_RE = /\[\[([^\]|#]+?)(?:#[^\]|]*?)?(?:\|[^\]]*?)?\]\]/g;
const EMBED_RE = /!\[\[([^\]|#]+?)(?:#[^\]|]*?)?(?:\|[^\]]*?)?\]\]/g;

function mdEscapeText(text: string): string {
  return text.replace(/([\\[\]])/g, '\\$1');
}

/** wikilink → 标准 markdown 链接（自定义协议承载跳转状态：wiki / wiki-missing / wiki-embed）。 */
function toWikiMd(label: string, target: string, heading: string, dangling: boolean, embed: boolean): string {
  const proto = embed ? 'kb-wiki-embed' : dangling ? 'kb-wiki-missing' : 'kb-wiki';
  const href = `${proto}://${encodeURIComponent(target)}${heading ? '#' + encodeURIComponent(heading) : ''}`;
  return `[${mdEscapeText(label)}](${href})`;
}

function transformSegment(seg: string): string {
  // embed 必须先于普通 wikilink 处理（共享前缀 ![[）。
  seg = seg.replace(EMBED_RE, (_m, raw: string) => {
    const target = raw.trim();
    if (!target) return _m;
    const dangling = !resolveWikiTarget(target, props.candidates ?? []);
    return toWikiMd(`📄 ${target}`, target, '', dangling, true);
  });
  return seg.replace(WIKILINK_RE, (m) => {
    const inner = m.slice(2, -2);
    const [head, alias = ''] = inner.split('|');
    const [targetRaw, headingRaw = ''] = (head ?? '').split('#');
    const target = (targetRaw ?? '').trim();
    if (!target) return m;
    const heading = headingRaw.trim();
    const label = (alias ?? '').trim() || target;
    const dangling = !resolveWikiTarget(target, props.candidates ?? []);
    return toWikiMd(label, target, heading, dangling, false);
  });
}

/** 围栏代码块（```/~~~）与行内单反引号 span 内不做替换（与编辑器装饰层同语义）。 */
function transformWikiSyntax(src: string): string {
  const lines = src.split('\n');
  let inFence = false;
  return lines
    .map((line) => {
      if (/^\s*(```|~~~)/.test(line)) {
        inFence = !inFence;
        return line;
      }
      if (inFence) return line;
      const segs = line.split('`');
      return segs.map((s, i) => (i % 2 === 0 && s ? transformSegment(s) : s)).join('`');
    })
    .join('\n');
}

// ---------- markdown-it 渲染 ----------

const md = new MarkdownIt({ html: false, linkify: true });
md.enable(['table', 'strikethrough']);

const defaultLinkOpen =
  md.renderer.rules.link_open ?? ((tokens, idx, options, _env, self) => self.renderToken(tokens, idx, options));

md.renderer.rules.link_open = (tokens, idx, options, env, self) => {
  const token = tokens[idx];
  if (!token) return defaultLinkOpen(tokens, idx, options, env, self);
  const href = token.attrGet('href') ?? '';
  if (href.startsWith('kb-wiki')) {
    const cls = href.startsWith('kb-wiki-missing')
      ? 'kb-wikilink kb-wikilink--dangling'
      : href.startsWith('kb-wiki-embed')
        ? 'kb-wikilink kb-wikilink--embed'
        : 'kb-wikilink';
    token.attrSet('class', cls);
    token.attrSet(
      'title',
      href.startsWith('kb-wiki-missing')
        ? t('knowledgePage.workbench.wikilinkDanglingPreview')
        : decodeURIComponent(href.slice(href.indexOf('://') + 3)),
    );
  } else if (/^https?:/i.test(href)) {
    token.attrSet('target', '_blank');
    token.attrSet('rel', 'noopener noreferrer');
  }
  return defaultLinkOpen(tokens, idx, options, env, self);
};

/** DOMPurify URI 白名单：http(s)/mailto/锚点/相对路径 + 自研 wikilink 协议。 */
const ALLOWED_URI_RE = /^(?:https?:|mailto:|tel:|kb-wiki(?:-missing|-embed)?:|#|[^/?#:]+(?:[/?#]|$))/i;

const renderedHtml = computed(() => {
  const transformed = transformWikiSyntax(props.content ?? '');
  const safe = DOMPurify.sanitize(md.render(transformed), {
    ALLOWED_URI_REGEXP: ALLOWED_URI_RE,
    ADD_ATTR: ['target', 'rel'],
  });
  // 任务列表复选框（消毒后注入自有标记）：紧凑/松散列表两种形态。
  return safe.replace(/<li>(?:<p>)?\[( |x|X)\]\s+/g, (m, mark: string) => {
    const open = m.startsWith('<li><p>') ? '<li><p class="kb-task-p">' : '<li>';
    const checked = mark.trim().toLowerCase() === 'x';
    return `${open}<span class="kb-task"><input type="checkbox" disabled${checked ? ' checked' : ''} /></span> `;
  });
});

// ---------- 交互 ----------

function onLinkClick(e: MouseEvent) {
  const a = (e.target as HTMLElement | null)?.closest('a');
  if (!a) return;
  const href = a.getAttribute('href') ?? '';
  if (!href.startsWith('kb-wiki')) return; // 外部链接 target=_blank 交给浏览器
  e.preventDefault();
  const body = href.slice(href.indexOf('://') + 3);
  const hashIdx = body.indexOf('#');
  const target = decodeURIComponent(hashIdx >= 0 ? body.slice(0, hashIdx) : body);
  const heading = hashIdx >= 0 ? decodeURIComponent(body.slice(hashIdx + 1)) : '';
  if (href.startsWith('kb-wiki-missing://')) {
    emit('create-doc', target);
    return;
  }
  if (heading) emit('open-doc', target, heading);
  else emit('open-doc', target);
}

/** 供右栏大纲面板滚动定位（SP2-5 对齐 NoteEditor 契约）：offset → 最近前述标题 → 定位 + 闪烁。 */
function scrollToOffset(offset: number) {
  const el = root.value;
  if (!el) return;
  const items = parseOutline(props.content ?? '').filter((h) => h.offset <= offset);
  const hit = items[items.length - 1];
  if (!hit) {
    el.scrollTo({ top: 0 });
    return;
  }
  const want = hit.text.trim();
  for (const h of Array.from(el.querySelectorAll('h1,h2,h3,h4,h5,h6'))) {
    h.classList.remove('kb-flash');
    if ((h.textContent ?? '').trim() === want) {
      h.scrollIntoView({ block: 'start', behavior: 'smooth' });
      h.classList.add('kb-flash');
      return;
    }
  }
}

/** G-17 引用锚定：raw 偏移 → 行号 → markdown-it 顶层块 token（level 0 且带 map）
 *  与渲染顶层子节点同序对齐 → 精确块级定位 + 闪烁；结构不一致退化为标题级定位。 */
function scrollToOffsetExact(offset: number) {
  const el = root.value;
  if (!el) return;
  const content = props.content ?? '';
  const pos = Math.min(Math.max(0, offset), Math.max(0, content.length - 1));
  const targetLine = content.slice(0, pos).split('\n').length - 1;
  const tokens = md.parse(transformWikiSyntax(content), {});
  const blocks = tokens.filter((tk) => tk.map && tk.level === 0 && tk.type !== 'inline' && !tk.type.endsWith('_close'));
  const wrap = el.querySelector('.kb-note-preview__content');
  const domBlocks = wrap ? Array.from(wrap.children) : [];
  if (!blocks.length || blocks.length !== domBlocks.length) {
    scrollToOffset(offset); // 消毒/注入改变了顶层结构时退化，仍给出标题级闪烁
    return;
  }
  let idx = 0;
  for (let i = 0; i < blocks.length; i++) {
    const start = blocks[i]?.map?.[0] ?? 0;
    if (start <= targetLine) idx = i;
    else break;
  }
  const hit = domBlocks[idx] as HTMLElement | undefined;
  if (!hit) {
    scrollToOffset(offset);
    return;
  }
  for (const h of Array.from(el.querySelectorAll('.kb-flash'))) h.classList.remove('kb-flash');
  hit.scrollIntoView({ block: 'start', behavior: 'smooth' });
  hit.classList.add('kb-flash');
}

/** 与 NoteEditor 契约对齐的 no-op（预览模式无挂起写回）。 */
function flushPendingContent() {}

defineExpose({ scrollToOffset, scrollToOffsetExact, flushPendingContent });
</script>

<style lang="sass" scoped>
.kb-note-preview
  flex: 1
  min-height: 0
  overflow-y: auto
  padding: 18px 22px 42px

  &__content
    max-width: 860px
    margin: 0 auto
    color: var(--kb-text-primary)
    font-size: 14px
    line-height: 1.85
    word-break: break-word

    :deep(h1), :deep(h2), :deep(h3), :deep(h4), :deep(h5), :deep(h6)
      margin: 1.1em 0 0.5em
      line-height: 1.35
      font-weight: 700

    :deep(h1), :deep(h2)
      color: var(--kb-accent-cyan)

    :deep(h1)
      font-size: 1.6em
      border-bottom: 1px solid var(--kb-line-hairline)
      padding-bottom: 0.25em

    :deep(h2)
      font-size: 1.35em

    :deep(h3)
      font-size: 1.18em
      font-weight: 600

    :deep(h4)
      font-size: 1.06em
      font-weight: 600

    :deep(p)
      margin: 0.55em 0

    :deep(a)
      color: var(--kb-accent-cyan)
      text-decoration: none

      &:hover
        text-decoration: underline

    :deep(code)
      background: rgba(var(--hud-sky-rgb), 0.08)
      border-radius: 4px
      padding: 1px 5px
      font-family: 'JetBrains Mono', 'Fira Code', Consolas, monospace
      font-size: 0.9em

    :deep(pre)
      background: rgba(var(--hud-sky-rgb), 0.05)
      border: 1px solid var(--kb-line-hairline)
      border-radius: 8px
      padding: 12px 14px
      overflow-x: auto
      margin: 0.7em 0

      code
        background: transparent
        padding: 0

    :deep(blockquote)
      margin: 0.7em 0
      border-left: 3px solid var(--kb-accent-cyan)
      padding: 2px 0 2px 12px
      color: var(--kb-text-dim)

    :deep(ul), :deep(ol)
      margin: 0.5em 0
      padding-left: 1.6em

    :deep(table)
      border-collapse: collapse
      margin: 0.8em 0
      width: 100%
      font-size: 0.94em

    :deep(th), :deep(td)
      border: 1px solid var(--kb-line-hairline)
      padding: 6px 10px
      text-align: left

    :deep(th)
      background: rgba(var(--hud-sky-rgb), 0.08)
      font-weight: 600

    :deep(tr:nth-child(even) td)
      background: rgba(var(--hud-sky-rgb), 0.03)

    :deep(img)
      max-width: 100%
      border-radius: 8px
      margin: 0.5em 0

    :deep(hr)
      border: 0
      border-top: 1px solid var(--kb-line-hairline)
      margin: 1.4em 0

    :deep(.kb-task input)
      margin-right: 6px
      vertical-align: -1px
      accent-color: var(--kb-accent-cyan)

    :deep(.kb-task-p)
      margin: 0

    // wikilink 芯片（与编辑器 Live Preview 同族视觉）
    :deep(.kb-wikilink)
      display: inline-block
      padding: 0 7px
      border-radius: 6px
      background: color-mix(in srgb, var(--kb-accent-cyan) 10%, transparent)
      border: 1px solid color-mix(in srgb, var(--kb-accent-cyan) 22%, transparent)
      color: var(--kb-accent-cyan)
      cursor: pointer
      transition: background 150ms ease-out, border-color 150ms ease-out, box-shadow 150ms ease-out
      text-decoration: none !important

      &:hover
        background: color-mix(in srgb, var(--kb-accent-cyan) 20%, transparent)
        border-color: color-mix(in srgb, var(--kb-accent-cyan) 38%, transparent)
        box-shadow: 0 0 12px color-mix(in srgb, var(--kb-accent-cyan) 25%, transparent)

    :deep(.kb-wikilink--dangling)
      color: var(--kb-text-faint)
      border: 1px dashed color-mix(in srgb, var(--kb-text-dim) 45%, transparent)
      background: color-mix(in srgb, var(--kb-text-dim) 8%, transparent)
      opacity: 0.75

      &:hover
        background: color-mix(in srgb, var(--kb-text-dim) 14%, transparent)
        border-color: color-mix(in srgb, var(--kb-text-dim) 60%, transparent)
        box-shadow: none
        opacity: 0.95

    :deep(.kb-wikilink--embed)
      border-style: solid
      font-style: italic

    // 大纲定位闪烁
    :deep(.kb-flash)
      animation: kb-flash-pulse 1.2s ease-out

@keyframes kb-flash-pulse
  0%
    background: color-mix(in srgb, var(--kb-accent-cyan) 30%, transparent)
  100%
    background: transparent
</style>
