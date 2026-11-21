import { describe, expect, it } from 'vitest';
import type { MailMessageRow } from '../../office/types';
import {
  attachmentSrcdoc,
  avatarTone,
  countBlockedImages,
  dateGroup,
  displayName,
  fileBadge,
  groupByDate,
  initial,
  listTime,
  mailSrcdoc,
  parseAddr,
  prefixedSubject,
  previewKind,
  quotedOriginal,
  replyRecipients,
  splitAddrs,
  syncSinceFor,
} from '../model';

function msg(over: Partial<MailMessageRow> = {}): MailMessageRow {
  return {
    id: 'm1',
    account_id: 'a1',
    subject: '合同第 3 条付款节点确认',
    sender: '李卓 <li@partner.cn>',
    snippet: '',
    received_at: new Date(2026, 10, 19, 15, 42).toISOString(),
    task_ref: '',
    has_attachments: false,
    to: '我 <me@company.cn>, 王敏 <wm@company.cn>',
    cc: 'boss@company.cn',
    seen: false,
    flagged: false,
    archived: false,
    folder: 'inbox',
    trashed: false,
    spam: false,
    labels: [],
    send_at: '',
    ...over,
  };
}

const NOW = new Date(2026, 10, 19, 18, 0); // 周四

describe('mail model', () => {
  it('解析地址：带名字、带引号、只有地址；按全角逗号 / 分号切，引号里的逗号不切', () => {
    expect(parseAddr('李卓 <li@partner.cn>')).toEqual({ name: '李卓', address: 'li@partner.cn' });
    expect(parseAddr('"Li, Zhuo" <li@partner.cn>')).toEqual({ name: 'Li, Zhuo', address: 'li@partner.cn' });
    expect(parseAddr('li@partner.cn')).toEqual({ name: '', address: 'li@partner.cn' });
    expect(splitAddrs('"Li, Zhuo" <li@p.cn>，wm@c.cn；x@y.cn')).toEqual(['"Li, Zhuo" <li@p.cn>', 'wm@c.cn', 'x@y.cn']);
    expect(displayName('noreply@github.com')).toBe('noreply');
    expect(initial('alice@x.com')).toBe('A');
    expect(avatarTone('A <a@x.com>')).toBe(avatarTone('a@x.com'));
  });

  it('按今天 / 昨天 / 本周 / 更早分组，列表时间今天只写时分', () => {
    const at = (d: number, h = 9) => new Date(2026, 10, d, h).toISOString();
    expect(dateGroup(at(19), NOW)).toBe('today');
    expect(dateGroup(at(18), NOW)).toBe('yesterday');
    expect(dateGroup(at(16), NOW)).toBe('week'); // 周一
    expect(dateGroup(at(15), NOW)).toBe('earlier');
    const groups = groupByDate(
      [msg({ received_at: at(19) }), msg({ received_at: at(19, 8) }), msg({ received_at: at(10) })],
      NOW,
    );
    expect(groups.map((g) => [g.key, g.rows.length])).toEqual([
      ['today', 2],
      ['earlier', 1],
    ]);
    expect(listTime(at(19, 9), NOW)).toBe('09:00');
    expect(listTime(at(3), NOW)).toBe('11-03');
    expect(listTime(new Date(2025, 0, 2).toISOString(), NOW)).toBe('2025-01-02');
  });

  it('回复 / 转发主题不叠前缀', () => {
    expect(prefixedSubject('合同', 'reply')).toBe('Re: 合同');
    expect(prefixedSubject('Re: 合同', 'replyAll')).toBe('Re: 合同');
    expect(prefixedSubject('回复：合同', 'reply')).toBe('回复：合同');
    expect(prefixedSubject('合同', 'forward')).toBe('Fwd: 合同');
    expect(prefixedSubject('转发: 合同', 'forward')).toBe('转发: 合同');
  });

  it('全部回复：原发件人 + 其他收件人进收件，抄送保留，去掉自己、去重', () => {
    const m = msg({ cc: 'boss@company.cn, WM@company.cn' });
    expect(replyRecipients(m, 'reply', ['me@company.cn'])).toEqual({ to: '李卓 <li@partner.cn>', cc: '' });
    expect(replyRecipients(m, 'replyAll', ['me@company.cn'])).toEqual({
      to: '李卓 <li@partner.cn>, 王敏 <wm@company.cn>',
      cc: 'boss@company.cn',
    });
    // 回复自己发出的信 = 接着发给原收件人
    const mine = msg({ sender: '我 <me@company.cn>', to: 'li@partner.cn', cc: '', folder: 'sent' });
    expect(replyRecipients(mine, 'reply', ['me@company.cn']).to).toBe('li@partner.cn');
  });

  it('引用原文逐行加 >', () => {
    const q = quotedOriginal(msg(), '第一行\n第二行', {
      header: '原始邮件',
      from: '发件人',
      to: '收件人',
      time: '时间',
      subject: '主题',
    });
    expect(q).toContain('—— 原始邮件 ——');
    expect(q).toContain('发件人: 李卓 <li@partner.cn>');
    expect(q.endsWith('> 第一行\n> 第二行')).toBe(true);
  });

  it('iframe 文档：默认只放行 data: 图片；点了显示图片才换回 src 并放行 https', () => {
    const html = '<p>hi</p><img data-mail-src="https://t.example/a.png">';
    const p = { ink: 'ink', paper: 'paper', link: 'link', muted: 'muted', font: 'sans' };
    expect(countBlockedImages(html)).toBe(1);
    const off = mailSrcdoc(html, false, p);
    expect(off).toContain('img-src data:;');
    expect(off).toContain('data-mail-src="https://t.example/a.png"');
    expect(off).toContain("default-src 'none'");
    const on = mailSrcdoc(html, true, p);
    expect(on).toContain('img-src data: https: http:');
    expect(on).toContain('<img src="https://t.example/a.png">');
  });

  it('附件类型和首次同步范围', () => {
    expect(fileBadge('合同.docx')).toBe('DOC');
    expect(fileBadge('报价.XLSX')).toBe('XLS');
    expect(fileBadge('scan', 'image/png')).toBe('IMG');
    expect(fileBadge('a.bin')).toBe('FILE');
    expect(syncSinceFor('all', NOW)).toBe('');
    expect(new Date(syncSinceFor('7d', NOW)).getTime()).toBe(NOW.getTime() - 7 * 86_400_000);
  });

  it('附件预览类型与 HTML 附件的隔离', () => {
    expect(previewKind('规则.html', 'text/html')).toBe('html');
    expect(previewKind('scan.PDF', 'application/octet-stream')).toBe('pdf');
    expect(previewKind('logo', 'image/png')).toBe('image');
    expect(previewKind('notes.csv')).toBe('text');
    expect(previewKind('素材.zip', 'application/zip')).toBe('none');
    const doc = attachmentSrcdoc('<script>x()</script><img src="https://t.example/a.png">');
    expect(doc).toContain("default-src 'none'");
    expect(doc).toContain('img-src data:;');
  });
});
