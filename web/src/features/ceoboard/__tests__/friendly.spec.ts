// 翻译层单测（整合 P2-8）：zh-CN 语境下输出必须与 ceo-dashboard 原版逐字一致
// （原版字符串字面量 → 现 i18n `ceoboard.*` 键，值不变）。
import { describe, expect, it } from 'vitest';
import { i18n } from '../../../i18n';
import {
  friendlyAlert,
  friendlyErr,
  friendlyReason,
  healthLabel,
  laneName,
  laneShortName,
  metricName,
  platformName,
} from '../friendly';

// 与原版一致的 zh-CN 断言前提。
i18n.global.locale.value = 'zh-CN';

describe('platformName / laneName', () => {
  it('已知平台走字典,未知平台回退声明名再回退 id', () => {
    expect(platformName('NovaTrader')).toBe('量化交易');
    expect(platformName('HotCraftFlow')).toBe('内容工厂');
    expect(platformName('x', 'HotCraftFlow')).toBe('内容工厂');
    expect(platformName('x', '自定义名')).toBe('自定义名');
    expect(platformName('x')).toBe('x');
  });

  it('业务线长/短名字典', () => {
    expect(laneName('content')).toBe('内容线');
    expect(laneName('invest')).toBe('投资线');
    expect(laneShortName('platform')).toBe('底座');
    expect(laneName('unknown-line')).toBe('unknown-line');
  });
});

describe('healthLabel', () => {
  it('四态标签,未知原样返回', () => {
    expect(healthLabel('red')).toBe('异常');
    expect(healthLabel('yellow')).toBe('需关注');
    expect(healthLabel('green')).toBe('正常');
    expect(healthLabel('gray')).toBe('无数据');
    expect(healthLabel('weird')).toBe('weird');
  });
});

describe('friendlyErr', () => {
  it('空输入归空串', () => {
    expect(friendlyErr(undefined)).toBe('');
    expect(friendlyErr('')).toBe('');
  });

  it('dial tcp 连接失败 → 人话', () => {
    expect(
      friendlyErr('Get "http://10.0.0.1:8080/x": dial tcp 10.0.0.1:8080: connectex: No connection could be made because the target machine actively refused it.'),
    ).toBe('无法连接 10.0.0.1:8080（服务未启动）');
  });

  it('超时/域名/HTTP 码翻译', () => {
    expect(friendlyErr('context deadline exceeded')).toBe('连接超时');
    expect(friendlyErr('no such host')).toBe('域名无法解析');
    expect(friendlyErr('HTTP 401')).toBe('接口鉴权失败（需重新授权）');
    expect(friendlyErr('HTTP 403')).toBe('接口无权限（HTTP 403）');
    expect(friendlyErr('HTTP 502')).toBe('对方服务内部错误（HTTP 502）');
  });

  it('NATS 消费类错误翻译', () => {
    expect(friendlyErr('durable consumer 未建立')).toBe('实时数据通道未建立');
    // 标点后的余串保留（与原版一致：替换不吞标点）。
    expect(friendlyErr('durable consumer 未建立,等待重试')).toBe('实时数据通道未建立,等待重试');
    expect(friendlyErr('全部 subject 消费建立失败')).toBe('所有消息订阅均未建立');
  });

  it('无法识别时截断至 80 字符', () => {
    const long = 'x'.repeat(120);
    const out = friendlyErr(long);
    expect(out.length).toBe(81);
    expect(out.endsWith('…')).toBe(true);
  });

  it('平台探针失败前缀剥落后翻译余串', () => {
    expect(friendlyErr('平台探针失败: no such host')).toBe('域名无法解析');
  });
});

describe('friendlyReason', () => {
  it('空 reason → 运行正常', () => {
    expect(friendlyReason(undefined)).toBe('运行正常');
    expect(friendlyReason('')).toBe('运行正常');
  });

  it('登录冷却中 → 一句话讲清', () => {
    expect(friendlyReason('x; 登录冷却中 300s')).toBe('接口鉴权失败，冷却等待自动重试中');
  });

  it('健康噪音段在存在异常段时丢弃', () => {
    expect(friendlyReason('durable 消费中(12); HTTP 502')).toBe('对方服务内部错误（HTTP 502）');
  });

  it('仅剩噪音段时保留通道运行中', () => {
    expect(friendlyReason('durable 消费中(3)')).toBe('实时数据通道运行中');
  });

  it('多异常段最多取前 2 段', () => {
    expect(friendlyReason('HTTP 502; HTTP 403; no such host')).toBe(
      '对方服务内部错误（HTTP 502）；接口无权限（HTTP 403）',
    );
  });
});

describe('friendlyAlert', () => {
  it('平台探针失败 → 离线标题', () => {
    const out = friendlyAlert({ target: 'NovaTrader', text: '平台探针失败: no such host' });
    expect(out.title).toBe('「量化交易」已离线');
    expect(out.detail).toBe('域名无法解析');
  });

  it('对账数据源不可达/对账失败 → 对应标题', () => {
    expect(friendlyAlert({ target: 'wf.a', text: '对账数据源不可达: timeout' }).title).toBe('wf.a 数据停止更新');
    expect(friendlyAlert({ target: 'wf.a', text: '对账失败(以此为准): HTTP 502' }).title).toBe('wf.a 数据核对失败');
  });

  it('其余告警原样透出 target', () => {
    expect(friendlyAlert({ target: 'wf.b', text: 'HTTP 502' }).title).toBe('wf.b');
  });
});

describe('metricName', () => {
  it('已知指标走字典,m. 与前缀剥离', () => {
    expect(metricName('m.throughput')).toBe('处理量');
    expect(metricName('n1.backlog')).toBe('积压');
    expect(metricName('rtt')).toBe('响应延迟');
  });

  it('未知指标下划线转空格', () => {
    expect(metricName('custom_metric_x')).toBe('custom metric x');
  });

  it('note 优先,超 16 字符截断;实现细节引用剔除', () => {
    expect(metricName('m.x', '简单备注')).toBe('简单备注');
    expect(metricName('m.x', '拦截口径（见 xxx.go 实现）')).toBe('拦截口径');
    // 「见 smoke.sh」引用整段剔除后为空 → 回退 id 路径（与原版一致）。
    expect(metricName('m.x', '见 smoke.sh 的说明')).toBe('x');
    const long = '这是一个非常非常长的指标备注说明文字';
    expect(metricName('m.x', long)).toBe(long.slice(0, 16) + '…');
  });
});
