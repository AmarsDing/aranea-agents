import { describe, expect, it } from 'vitest';
import { commandTitle, parseCommand, planPaletteActions, sessionLink, type Recipient } from '../command';

const recipients: Recipient[] = [
  { kind: 'agent', id: 'ag-1', key: 'content', name: '内容组' },
  { kind: 'agent', id: 'ag-2', key: 'content_review', name: '内容组评审岗' },
  { kind: 'team', id: 'tm-1', key: 'trade', name: 'Trade Desk' },
];

describe('parseCommand', () => {
  it('defaults to a task', () => {
    expect(parseCommand('  写 Q4 复盘提纲 ', recipients)).toEqual({
      intent: 'task',
      text: '写 Q4 复盘提纲',
      recipientRef: '',
    });
    expect(parseCommand('   ', recipients)).toEqual({ intent: 'task', text: '', recipientRef: '' });
  });

  it('recognises notes only with a clear prefix', () => {
    expect(parseCommand('记：王总说合同月底前签', recipients)).toMatchObject({
      intent: 'note',
      text: '王总说合同月底前签',
    });
    expect(parseCommand('记一下供应商报价 3 万', recipients)).toMatchObject({
      intent: 'note',
      text: '供应商报价 3 万',
    });
    expect(parseCommand('note: call back', recipients)).toMatchObject({ intent: 'note', text: 'call back' });
    // 「记得…」是要做的事，不是记录
    expect(parseCommand('记得买票', recipients)).toMatchObject({ intent: 'task', text: '记得买票' });
  });

  it('delegates when a verb is followed by a known recipient', () => {
    expect(parseCommand('让内容组出一份竞品周报，周三前', recipients)).toEqual({
      intent: 'delegate',
      text: '出一份竞品周报，周三前',
      recipientRef: 'agent:ag-1',
    });
    expect(parseCommand('交给 trade desk：复核今天的调仓', recipients)).toEqual({
      intent: 'delegate',
      text: '复核今天的调仓',
      recipientRef: 'team:tm-1',
    });
  });

  it('prefers the longest matching name', () => {
    expect(parseCommand('请内容组评审岗看一下这篇稿子', recipients)).toMatchObject({
      recipientRef: 'agent:ag-2',
      text: '看一下这篇稿子',
    });
    expect(parseCommand('@content_review 看稿', recipients)).toMatchObject({
      recipientRef: 'agent:ag-2',
      text: '看稿',
    });
  });

  it('keeps verb sentences without a recipient as tasks', () => {
    expect(parseCommand('让我想想怎么回复王总', recipients)).toMatchObject({ intent: 'task', recipientRef: '' });
    expect(parseCommand('请假条写一下', recipients)).toMatchObject({ intent: 'task' });
  });

  it('treats @ as delegation even when the name is unknown', () => {
    expect(parseCommand('@谁都行 查一下库存', recipients)).toEqual({
      intent: 'delegate',
      text: '谁都行 查一下库存',
      recipientRef: '',
    });
  });
});

describe('commandTitle', () => {
  it('uses the first line and clips', () => {
    expect(commandTitle('第一行\n第二行')).toBe('第一行');
    expect(commandTitle('长'.repeat(80))).toBe(`${'长'.repeat(60)}…`);
  });
});

describe('sessionLink', () => {
  it('points at the chat session of the recipient', () => {
    expect(sessionLink({ kind: 'agent', id: 'ag 1' }, 's/1')).toBe('/chat?session=s%2F1&agent=ag%201');
    expect(sessionLink({ kind: 'team', id: 'tm-1' }, 's1')).toBe('/chat?session=s1&team=tm-1');
  });
});

describe('planPaletteActions', () => {
  it('offers nothing for an empty query', () => {
    expect(planPaletteActions('  ', recipients)).toEqual({ actions: [], actionsFirst: false });
  });

  it('puts delegation first when the sentence names a recipient', () => {
    const plan = planPaletteActions('让内容组出一份竞品周报', recipients);
    expect(plan.actionsFirst).toBe(true);
    expect(plan.actions[0]).toMatchObject({ kind: 'delegate', text: '出一份竞品周报', recipient: { id: 'ag-1' } });
    expect(plan.actions[1]).toEqual({ kind: 'task', text: '让内容组出一份竞品周报' });
  });

  it('suggests recipients for a half-typed @ name', () => {
    const plan = planPaletteActions('@内容 看一下这篇稿子', recipients);
    expect(plan.actions.map((a) => (a.kind === 'delegate' ? a.recipient.id : a.kind))).toEqual(['ag-1', 'ag-2']);
    expect(plan.actions[0]).toMatchObject({ text: '看一下这篇稿子' });
    expect(planPaletteActions('@内容', recipients).actions).toEqual([]);
  });

  it('puts notes first for a note prefix', () => {
    const plan = planPaletteActions('记：王总说合同月底前签', recipients);
    expect(plan).toEqual({
      actions: [
        { kind: 'note', text: '王总说合同月底前签' },
        { kind: 'task', text: '记：王总说合同月底前签' },
      ],
      actionsFirst: true,
    });
  });

  it('keeps page jumps first for ordinary text, with task and note offered after', () => {
    const plan = planPaletteActions('工具', recipients);
    expect(plan.actionsFirst).toBe(false);
    expect(plan.actions.map((a) => a.kind)).toEqual(['task', 'note']);
  });
});
