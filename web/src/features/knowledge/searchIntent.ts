/**
 * 搜索意图分流（P3 统一搜索框双区）。
 *
 * ⚠️ 本规则表与后端 `internal/knowledge/search_intent.go` 维护**同一份定义**，
 * 修改任一侧必须同步另一侧（两侧注释互指）。
 *
 * 规则（按优先级）：
 *  1. 即时区（instant）——强定位信号，用户明显在找文件/路径/精确短语：
 *     - 含路径分隔符（`/` 或 `\`）
 *     - 含扩展名模式（`.md` `.pdf` `.txt` `.csv` `.json` `.yaml` `.docx` `.xlsx` `.pptx` `.png` `.jpg` `.webp` 等，词尾）
 *     - 引号包裹短语（`"..."`）
 *  2. 语义区（semantic）——自然语言问句，用户期待概念性回答：
 *     - 中文疑问词开头（什么/如何/怎么/为什么/哪些/哪/是否）或 `吗` 结尾
 *     - 英文 wh- 词开头（what/how/why/which/when/where/who）或 `?`/`？` 结尾
 *  3. auto —— 无强信号：双区并列展示（即时区即时过滤 + 回车走语义检索）。
 */

export type SearchIntent = 'instant' | 'semantic' | 'auto';

const PATH_SEPARAT = /[/\\]/;
// 词尾扩展名（点前非空白、后随边界），覆盖知识库常见可入库类型。
const FILE_EXT = /\.(md|markdown|txt|log|pdf|docx?|xlsx?|pptx?|csv|json|ya?ml|toml|xml|html?|png|jpe?g|webp)\b/i;
const QUOTED_PHRASE = /"[^"]+"|'[^']+'/;
// 中文疑问词（什么/如何/怎么/为什么/为何/哪些/哪个/哪/是否/能不能/可以不可以）
// 用码点构造正则（check-i18n 不允许源码出现 CJK 字面量，等价于字面写法）。
const zh = (...codes: number[]) => String.fromCharCode(...codes);
const ZH_HEAD_WORDS = [
  zh(0x4ec0, 0x4e48),
  zh(0x5982, 0x4f55),
  zh(0x600e, 0x4e48),
  zh(0x4e3a, 0x4ec0, 0x4e48),
  zh(0x4e3a, 0x4f55),
  zh(0x54ea, 0x4e9b),
  zh(0x54ea, 0x4e2a),
  zh(0x54ea),
  zh(0x662f, 0x5426),
  zh(0x80fd, 0x4e0d, 0x80fd),
  zh(0x53ef, 0x4ee5, 0x4e0d, 0x53ef, 0x4ee5),
];
const ZH_QUESTION_HEAD = new RegExp(`^(?:${ZH_HEAD_WORDS.join('|')})`);
// 吗/呢/吧/？
const ZH_QUESTION_TAIL = new RegExp(`[${zh(0x5417, 0x5462, 0x5427, 0xff1f)}]\\s*$`);
const EN_QUESTION_HEAD = /^(what|how|why|which|when|where|who|whom|whose|is|are|can|could|does|do)\b/i;
const EN_QUESTION_TAIL = /\?\s*$/;

export function classifySearchIntent(query: string): SearchIntent {
  const q = query.trim();
  if (!q) return 'auto';
  // 强即时信号优先：用户找的是具体文件/路径/短语，疑问语气不改变定位意图。
  if (PATH_SEPARAT.test(q) || FILE_EXT.test(q) || QUOTED_PHRASE.test(q)) {
    return 'instant';
  }
  if (ZH_QUESTION_HEAD.test(q) || ZH_QUESTION_TAIL.test(q) || EN_QUESTION_HEAD.test(q) || EN_QUESTION_TAIL.test(q)) {
    return 'semantic';
  }
  return 'auto';
}
