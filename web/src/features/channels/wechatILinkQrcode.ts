import QRCode from 'qrcode';

/**
 * iLink get_bot_qrcode 返回的 qrcode_img_content 语义是「待编码的扫码内容」
 * （当前实际为 https://liteapp.weixin.qq.com/q/... 的 HTML 页面 URL），而非图片本身，
 * 直接塞进 <img src> 会裂图。这里统一转换为 <img> 可渲染的 data URL：
 * - 已是 data: URL 的原样透传（兼容未来 iLink 直接返图）
 * - 否则按内容生成 SVG 二维码 data URL（SVG 无需 canvas，测试环境可跑）
 */
export async function resolveWechatILinkQrcodeDataUrl(content: string): Promise<string> {
  const trimmed = content.trim();
  if (!trimmed) return '';
  if (trimmed.startsWith('data:')) return trimmed;
  const svg = await QRCode.toString(trimmed, { type: 'svg', margin: 1, width: 320 });
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`;
}
