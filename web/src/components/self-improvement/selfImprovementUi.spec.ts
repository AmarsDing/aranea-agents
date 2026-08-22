import { describe, it, expect } from 'vitest';
import { resolveSIErrorMessage, siGateColor, siGateIcon } from './selfImprovementUi';

describe('resolveSIErrorMessage', () => {
  it('优先使用后端 Kratos envelope 的 message（如 409 冲突）', () => {
    const axiosLike = {
      response: { status: 409, data: { code: 409, message: 'run not in controllable state' } },
      message: 'Request failed with status code 409',
    };
    expect(resolveSIErrorMessage(axiosLike)).toBe('run not in controllable state');
  });

  it('envelope message 非字符串时回退 Error.message', () => {
    const axiosLike = {
      response: { status: 500, data: { code: 500 } },
      message: 'Request failed with status code 500',
    };
    expect(resolveSIErrorMessage(axiosLike)).toBe('Request failed with status code 500');
  });

  it('普通 Error 返回其 message', () => {
    expect(resolveSIErrorMessage(new Error('网络超时'))).toBe('网络超时');
  });

  it('非 Error 值字符串化', () => {
    expect(resolveSIErrorMessage('plain failure')).toBe('plain failure');
    expect(resolveSIErrorMessage(42)).toBe('42');
  });
});

describe('siGate presentation', () => {
  it('skipped 门禁用中性图标，不显示通过/失败', () => {
    expect(siGateIcon(false, true)).toBe('remove_circle_outline');
    expect(siGateColor(false, true)).toBe('grey-6');
    expect(siGateIcon(true, false)).toBe('check_circle');
    expect(siGateIcon(false, false)).toBe('cancel');
  });
});
