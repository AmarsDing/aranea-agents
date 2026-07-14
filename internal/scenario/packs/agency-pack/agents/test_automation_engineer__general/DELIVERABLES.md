## 📋 你的技术交付物
### 确定性 Playwright 测试（无 sleep、API 设置、角色选择器）

```typescript
import { test, expect } from './fixtures';

test('customer can complete checkout', async ({ page, api }) => {
  // 通过 API 设置——快速、确定性、并行安全
  const user = await api.createUser({ plan: 'free' });
  const product = await api.createProduct({ name: 'Widget', priceCents: 4999 });
  await page.context().addCookies(await api.sessionCookiesFor(user));

  await page.goto(`/products/${product.slug}`);

  // 基于角色的选择器能扛重构；自动等待断言取代 sleep
  await page.getByRole('button', { name: 'Add to cart' }).click();
  await page.getByRole('link', { name: 'Checkout' }).click();

  // 等待关键的网络响应，而非等待时间
  const orderResponse = page.waitForResponse(
    (r) => r.url().includes('/api/orders') && r.status() === 201
  );
  await page.getByRole('button', { name: 'Place order' }).click();
  await orderResponse;

  // Web-first 断言：重试直到为真或超时——无需手动轮询
  await expect(page.getByRole('heading', { name: 'Order confirmed' })).toBeVisible();
  await expect(page.getByTestId('order-total')).toHaveText('$49.99');
});
```

### Worker 级 Auth Fixture（登录一次，而非 200 次）

```typescript
// fixtures.ts——每个 worker 通过 API 认证一次，然后复用
import { test as base } from '@playwright/test';
import { ApiClient } from './api-client';

export const test = base.extend<{ api: ApiClient }, { workerStorageState: string }>({
  api: async ({}, use) => {
    await use(new ApiClient(process.env.API_URL!));
  },
  workerStorageState: [
    async ({}, use, workerInfo) => {
      const fileName = `.auth/worker-${workerInfo.workerIndex}.json`;
      const api = new ApiClient(process.env.API_URL!);
      // 每个 worker 独有用户：并行运行绝不共享状态
      const user = await api.createUser({ email: `w${workerInfo.workerIndex}@test.local` });
      await api.saveStorageState(user, fileName);
      await use(fileName);
    },
    { scope: 'worker' },
  ],
  storageState: ({ workerStorageState }, use) => use(workerStorageState),
});
```

### CI：分片、Trace、合并阻断（GitHub Actions）

```yaml
jobs:
  e2e:
    strategy:
      fail-fast: false
      matrix:
        shard: [1/4, 2/4, 3/4, 4/4]
    steps:
      - uses: actions/checkout@v4
      - run: npm ci && npx playwright install --with-deps chromium
      - run: npx playwright test --shard=${{ matrix.shard }}
        env:
          # 首次重试时 trace：绿构建零开销，红构建完整取证
          PLAYWRIGHT_TRACE: on-first-retry
      - uses: actions/upload-artifact@v4
        if: failure()
        with:
          name: traces-${{ strategy.job-index }}
          path: test-results/          # 每次失败的 trace、截图、视频
```

### Flake 分诊表

| 症状 | 可能根因 | 修复（而非变通）|
|---------|-------------------|------------------------------|
| 本地通过，CI 失败 | 时序：CI 更慢，暴露竞态 | 将基于时间的等待替换为基于条件的等待；审计 `waitForTimeout` |
| 仅并行运行时失败 | 共享状态：跨测试使用同一用户/记录 | 通过 API 工厂实现每测试或每 worker 独有数据 |
| 约 20 次失败 1 次，element-not-found | 动画/渲染竞态，不稳定选择器 | 对最终状态使用 web-first 断言；角色/test-id 选择器 |
| "无关"合并后失败 | 对应用级 fixture/种子数据的隐藏依赖 | 让测试拥有自己的数据；删除共享种子依赖 |
| 导航超时 | 第三方脚本/分析阻塞加载 | 在测试配置中阻断第三方路由；等待 app-ready 信号，而非 `load` |
