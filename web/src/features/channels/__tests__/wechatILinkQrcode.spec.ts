import { describe, expect, it } from 'vitest';
import { resolveWechatILinkQrcodeDataUrl } from '../wechatILinkQrcode';

describe('resolveWechatILinkQrcodeDataUrl', () => {
  it('passes through an existing data URL', async () => {
    const dataUrl = 'data:image/png;base64,iVBORw0KGgo=';
    await expect(resolveWechatILinkQrcodeDataUrl(dataUrl)).resolves.toBe(dataUrl);
  });

  it('returns empty string for blank content', async () => {
    await expect(resolveWechatILinkQrcodeDataUrl('')).resolves.toBe('');
    await expect(resolveWechatILinkQrcodeDataUrl('   ')).resolves.toBe('');
  });

  it('encodes a scan-target URL into an SVG data URL', async () => {
    const content = 'https://liteapp.weixin.qq.com/q/7GiQu1?qrcode=abc123&bot_type=3';
    const result = await resolveWechatILinkQrcodeDataUrl(content);
    expect(result.startsWith('data:image/svg+xml')).toBe(true);
    const svg = decodeURIComponent(result.slice(result.indexOf(',') + 1));
    expect(svg).toContain('<svg');
    expect(svg).toContain('</svg>');
  });
});
