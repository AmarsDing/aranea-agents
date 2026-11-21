import { describe, expect, it } from 'vitest';
import { deptOptions, flowInputs, samePath } from '../assign';
import type { PlatformResourceTreeNode } from '../../platform/types';

const node = (id: string, level: string, name: string, children: PlatformResourceTreeNode[] = []) =>
  ({ id, level, name, children }) as PlatformResourceTreeNode;

describe('deptOptions', () => {
  const sales = node('d1', 'department', '销售部', [
    node('p1', 'position', '客户经理'),
    node('p2', 'position', '售后'),
  ]);

  it('lists departments with every node under them, so agents on positions or on the department itself match', () => {
    const [opt] = deptOptions([node('c1', 'company', '甲公司', [sales])]);
    expect(opt).toMatchObject({ value: 'd1', label: '销售部' });
    expect([...opt!.nodes]).toEqual(['d1', 'p1', 'p2']);
  });

  it('prefixes the company name only when there is more than one company', () => {
    const tree = [
      node('c1', 'company', '甲公司', [sales]),
      node('c2', 'company', '乙公司', [node('d2', 'department', '财务部')]),
    ];
    expect(deptOptions(tree).map((o) => o.label)).toEqual(['甲公司 › 销售部', '乙公司 › 财务部']);
  });

  it('ignores trees without companies', () => {
    expect(deptOptions([node('x', 'industry', '行业')])).toEqual([]);
  });
});

describe('flowInputs', () => {
  it('asks only for required string fields the runtime does not fill itself', () => {
    expect(
      flowInputs([
        { name: 'user_input', type: 'string', required: true },
        { name: 'messages', type: 'messages', required: true },
        { name: 'version', type: 'string', required: true },
        { name: 'note', type: 'string', required: false },
        { name: 'count', type: 'int', required: true },
      ]),
    ).toEqual(['version']);
  });
});

describe('samePath', () => {
  it('treats slash direction, case and a trailing slash as the same folder', () => {
    expect(samePath('D:\\Code\\Shop\\', 'd:/code/shop')).toBe(true);
    expect(samePath('D:\\code\\shop', 'D:\\code\\shop2')).toBe(false);
    expect(samePath('', '')).toBe(false);
  });
});
