// 业务语言翻译层（整合 P2-8，自 ceo-dashboard web/src/friendly.ts 语义等价移植）：
// 把工程细节（Go 报错、适配器名、指标 ID）翻译成管理者能读懂的话。
// 原始信息不丢弃——保留在 tooltip 和值班视图/抽屉里，老板视图只呈现结论。
//
// 文案口径：UI 文案（替换模板/字典）走 i18n `ceoboard.*` 命名空间；
// 正则中的中文是**后端中文错误串的协议字面值匹配**（不是 UI 文案），按
// scripts/check-i18n.mjs 的豁免规则逐行标记 i18n-exempt。

import { i18n } from '../../i18n';

function t(key: string, named?: Record<string, unknown>): string {
  return i18n.global.t(key, named ?? {});
}
function dictName(kind: 'platform' | 'lane' | 'laneShort' | 'metric', id: string): string | undefined {
  const key = `ceoboard.dict.${kind}.${id}`;
  return i18n.global.te(key) ? i18n.global.t(key) : undefined;
}

// ---- 名称映射 ----
export function platformName(id: string, name?: string): string {
  return dictName('platform', id) ?? dictName('platform', name ?? '') ?? name ?? id;
}

export function laneName(k: string): string {
  return dictName('lane', k) ?? k;
}

/** LaneGroup 短名（值班视图泳道头）。 */
export function laneShortName(k: string): string {
  return dictName('laneShort', k) ?? k;
}

// ---- 健康度 ----
export function healthLabel(h: string): string {
  const key = `ceoboard.health.${h}`;
  return i18n.global.te(key) ? i18n.global.t(key) : h;
}

// ---- 错误翻译 ----
// 输入任意工程错误串，输出一句人话；无法识别时截断原串。
export function friendlyErr(raw?: string): string {
  if (!raw) return '';
  let s = raw;
  s = s.replace(
    /Get "https?:\/\/[^"]*": dial tcp ([\d.:a-zA-Z-]+): connectex: No connection could be made[^,。]*/g,
    (_m, addr: string) => t('ceoboard.err.unreachable', { addr }),
  );
  s = s.replace(/dial tcp ([\d.:a-zA-Z-]+): connectex: No connection could be made[^,。]*/g, (_m, addr: string) =>
    t('ceoboard.err.unreachable', { addr }),
  );
  s = s.replace(/Get "https?:\/\/[^"]*"/g, () => t('ceoboard.err.requestApi'));
  s = s.replace(/context deadline exceeded|Client\.Timeout exceeded|timeout/gi, () => t('ceoboard.err.timeout'));
  s = s.replace(/no such host/gi, () => t('ceoboard.err.noSuchHost'));
  s = s.replace(/凭据获取失败[:：]?\s*登录被拒[:：]?\s*/g, ''); // i18n-exempt 匹配后端中文错误串（协议字面值）
  s = s.replace(/HTTP 401/, () => t('ceoboard.err.http401'));
  s = s.replace(/HTTP 403/, () => t('ceoboard.err.http403'));
  s = s.replace(/HTTP (5\d\d)/, (_m, code: string) => t('ceoboard.err.http5xx', { code }));
  s = s.replace(/durable consumer 未建立[^,，。]*/g, () => t('ceoboard.err.consumerMissing')); // i18n-exempt 匹配后端中文错误串
  s = s.replace(/全部 subject 消费建立失败/g, () => t('ceoboard.err.allSubscribeFailed')); // i18n-exempt 匹配后端中文错误串
  s = s.replace(/对账失败\(以此为准\)[:：]?\s*/g, ''); // i18n-exempt 匹配后端中文错误串
  s = s.replace(/对账数据源不可达[:：]?\s*/g, ''); // i18n-exempt 匹配后端中文错误串
  s = s.replace(/平台探针失败[:：]?\s*/g, ''); // i18n-exempt 匹配后端中文错误串
  s = s.replace(/探针失败[:：]?\s*/g, ''); // i18n-exempt 匹配后端中文错误串
  s = s.trim().replace(/^[:：]\s*/, '');
  return s.length > 80 ? s.slice(0, 80) + '…' : s;
}

// 节点状态说明 → 老板视图一句话
export function friendlyReason(reason?: string): string {
  const channelRunning = t('ceoboard.err.channelRunning');
  if (!reason) return t('ceoboard.reason.ok');
  // 登录冷却中的场景：一句话讲清
  if (/登录冷却中/.test(reason)) return t('ceoboard.reason.coolingDown'); // i18n-exempt 匹配后端中文状态串
  // 按分号拆段翻译；健康噪音段（实时通道运行中）在存在异常段时丢弃
  const segs = reason
    .split(/[;；]/)
    .map((seg) => {
      let s = seg.replace(/durable 消费中[(（][^)）]*[)）]?/g, channelRunning); // i18n-exempt 匹配后端中文状态串
      s = s.replace(/探针失败[:：]?\s*/g, ''); // i18n-exempt 匹配后端中文错误串
      // 去掉实现细节引用（文件名、脚本名）
      s = s.replace(/\s*见 smoke\.sh[^)）]*/g, ''); // i18n-exempt 匹配后端注释引用
      return friendlyErr(s).trim();
    })
    .filter((s) => s);
  const noisy = segs.filter((s) => s !== channelRunning);
  if (!noisy.length) return channelRunning;
  return noisy.slice(0, 2).join('；');
}

// 告警 → 老板视图 { 标题, 说明 }
export function friendlyAlert(a: { target: string; text: string }): { title: string; detail: string } {
  const text = a.text;
  const who = platformName(a.target);
  if (/平台探针失败/.test(text)) { // i18n-exempt 匹配后端中文告警文案
    return {
      title: t('ceoboard.alert.offlineTitle', { who }),
      detail: friendlyErr(text).replace(/^平台探针失败[:：]?\s*/, ''), // i18n-exempt 匹配后端中文告警文案
    };
  }
  if (/对账数据源不可达/.test(text)) { // i18n-exempt 匹配后端中文告警文案
    return { title: t('ceoboard.alert.staleTitle', { target: a.target }), detail: friendlyErr(text) };
  }
  if (/对账失败/.test(text)) { // i18n-exempt 匹配后端中文告警文案
    return { title: t('ceoboard.alert.reconFailedTitle', { target: a.target }), detail: friendlyErr(text) };
  }
  return { title: a.target, detail: friendlyErr(text) };
}

// ---- 指标名翻译 ----
export function metricName(id: string, note?: string): string {
  if (note) {
    const clean = note
      .replace(/[;；]?\s*见 smoke\.sh[^)）]*/g, '') // i18n-exempt 匹配后端注释引用
      .replace(/[（(][^)）]*\.go[^)）]*[)）]/g, '')
      .trim();
    if (clean) return clean.length > 16 ? clean.slice(0, 16) + '…' : clean;
  }
  const short = id.replace(/^m\./, '').replace(/^[a-z]+\d*\./i, '');
  return dictName('metric', short) ?? short.replace(/_/g, ' ');
}
