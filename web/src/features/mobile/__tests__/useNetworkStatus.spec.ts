import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { effectScope, type EffectScope } from 'vue';
import { useNetworkStatus } from '../useNetworkStatus';

let onlineFlag = true;

beforeEach(() => {
  onlineFlag = true;
  Object.defineProperty(window.navigator, 'onLine', {
    configurable: true,
    get: () => onlineFlag,
  });
});

afterEach(() => {
  Object.defineProperty(window.navigator, 'onLine', {
    configurable: true,
    value: true,
  });
});

describe('useNetworkStatus', () => {
  it('initialises from navigator.onLine', () => {
    const scope: EffectScope = effectScope();
    scope.run(() => {
      expect(useNetworkStatus().online.value).toBe(true);
    });
    scope.stop();

    onlineFlag = false;
    const scope2: EffectScope = effectScope();
    scope2.run(() => {
      expect(useNetworkStatus().online.value).toBe(false);
    });
    scope2.stop();
  });

  it('tracks online/offline window events', () => {
    const scope: EffectScope = effectScope();
    let online!: ReturnType<typeof useNetworkStatus>['online'];
    scope.run(() => {
      online = useNetworkStatus().online;
    });
    window.dispatchEvent(new Event('offline'));
    expect(online.value).toBe(false);
    window.dispatchEvent(new Event('online'));
    expect(online.value).toBe(true);
    scope.stop();
  });

  it('removes listeners when the owning scope is disposed', () => {
    const scope: EffectScope = effectScope();
    let online!: ReturnType<typeof useNetworkStatus>['online'];
    scope.run(() => {
      online = useNetworkStatus().online;
    });
    scope.stop();
    window.dispatchEvent(new Event('offline'));
    expect(online.value).toBe(true);
  });
});
