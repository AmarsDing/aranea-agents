import { describe, expect, it } from 'vitest';
import { fzfScore, instantFilter } from '../instantMatch';

describe('fzfScore', () => {
    it('子序列命中返回正分', () => {
        expect(fzfScore('rpt', 'report.md')).toBeGreaterThan(0);
        expect(fzfScore('财报', '2026 财报.pdf')).toBeGreaterThan(0);
    });

    it('非子序列返回 -1', () => {
        expect(fzfScore('xyz', 'report.md')).toBe(-1);
        expect(fzfScore('ab', 'ba')).toBe(-1);
    });

    it('大小写不敏感', () => {
        expect(fzfScore('READ', 'readme.md')).toBeGreaterThan(0);
    });

    it('连续命中得分高于分散命中', () => {
        const consecutive = fzfScore('abc', 'abc.txt');
        const scattered = fzfScore('abc', 'a-b-c.txt');
        expect(consecutive).toBeGreaterThan(scattered);
    });

    it('词边界命中加分', () => {
        const boundary = fzfScore('md', 'notes/report.md');
        const inline = fzfScore('md', 'middleware');
        expect(boundary).toBeGreaterThan(inline);
    });

    it('空 query 得 0 分', () => {
        expect(fzfScore('', 'anything')).toBe(0);
    });
});

describe('instantFilter', () => {
    const docs = [
        { name: 'report.md', path: 'notes/report.md', tags: '财报 季度' },
        { name: 'readme.md', path: 'readme.md', tags: '' },
        { name: 'refund.md', path: 'policies/refund.md', tags: '退款 政策' },
    ];
    const keys = (d: (typeof docs)[number]) => [d.name, d.path, d.tags];

    it('按得分降序返回，最相关在前', () => {
        const out = instantFilter(docs, 'report', keys);
        expect(out.map((d) => d.name)).toEqual(['report.md']);
    });

    it('多词条 AND：每个词都必须命中', () => {
        expect(instantFilter(docs, 'notes report', keys).map((d) => d.name)).toEqual(['report.md']);
        expect(instantFilter(docs, 'notes refund', keys)).toEqual([]);
    });

    it('可命中 tags 字段', () => {
        expect(instantFilter(docs, '退款', keys).map((d) => d.name)).toEqual(['refund.md']);
    });

    it('空 query 返回空数组', () => {
        expect(instantFilter(docs, '   ', keys)).toEqual([]);
    });

    it('limit 截断', () => {
        const many = Array.from({ length: 30 }, (_, i) => ({ name: `doc${i}.md`, path: `doc${i}.md`, tags: '' }));
        expect(instantFilter(many, 'doc', keys, 10)).toHaveLength(10);
    });
});
