// outline / frontmatter 纯函数单测（SP2 §SP2-8）。
import { describe, expect, it } from 'vitest';
import { parseOutline } from '../outline';
import { parseFrontmatter } from '../frontmatter';

describe('parseOutline', () => {
  it('parses ATX headings with offsets and strips closing #', () => {
    const doc = '# 一级\n\n正文\n## 二级 ##\n### 三级\n';
    const items = parseOutline(doc);
    expect(items).toHaveLength(3);
    expect(items[0]).toMatchObject({ level: 1, text: '一级', line: 0, offset: 0 });
    expect(items[1]).toMatchObject({ level: 2, text: '二级', line: 3 });
    expect(items[2]).toMatchObject({ level: 3, text: '三级', line: 4 });
    // offset 指向行首
    expect(doc.slice(items[1].offset, items[1].offset + 2)).toBe('##');
  });

  it('ignores headings inside code fences', () => {
    const doc = '# 真标题\n```\n# 注释不是标题\n```\n## 真标题二\n~~~\n# 也不是\n~~~\n';
    const items = parseOutline(doc);
    expect(items.map((x) => x.text)).toEqual(['真标题', '真标题二']);
  });

  it('returns empty for no headings', () => {
    expect(parseOutline('纯文本\n无标题')).toEqual([]);
  });
});

describe('parseFrontmatter', () => {
  it('parses scalar, inline list and block list', () => {
    const doc = '---\ntitle: 笔记\ntags: [a, b]\naliases:\n  - 别名一\n  - 别名二\n---\n正文';
    const fm = parseFrontmatter(doc);
    expect(fm).not.toBeNull();
    expect(fm?.entries).toHaveLength(3);
    expect(fm?.entries[0]).toMatchObject({ key: 'title', value: '笔记' });
    expect(fm?.entries[1]).toMatchObject({ key: 'tags', value: 'a, b', values: ['a', 'b'] });
    expect(fm?.entries[2]).toMatchObject({ key: 'aliases', values: ['别名一', '别名二'] });
  });

  it('returns null without frontmatter section', () => {
    expect(parseFrontmatter('# 直接标题')).toBeNull();
    expect(parseFrontmatter('--- 没有换行')).toBeNull();
  });

  it('skips comments and blank lines', () => {
    const fm = parseFrontmatter('---\n# 注释\n\nkey: v\n---\n');
    expect(fm?.entries).toEqual([{ key: 'key', value: 'v', values: [] }]);
  });
});
