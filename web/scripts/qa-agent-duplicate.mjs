import { chromium } from 'playwright';
import { writeFileSync, mkdirSync, appendFileSync } from 'fs';
import { join } from 'path';

const BASE_URL = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:9001';
const OUT_DIR = join(process.cwd(), '..', '.dogfood-output');
mkdirSync(OUT_DIR, { recursive: true });
const RUN_LOG = join(OUT_DIR, 'agent-duplicate.log');
writeFileSync(RUN_LOG, '');

function log(msg) {
  console.log(msg);
  appendFileSync(RUN_LOG, msg + '\n');
}

(async () => {
  const browser = await chromium.launch({
    headless: true,
    executablePath: process.env.CHROME_EXE || 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await context.newPage();

  const logs = [];
  const network = [];

  page.on('console', (msg) => {
    const type = msg.type();
    const text = msg.text();
    if (type === 'error' || type === 'warning') {
      // Ignore transient WebSocket probe connection warnings.
      if (/ws:\/\/.*\/v1\/ws.*probe=1.*closed before the connection is established/i.test(text)) {
        return;
      }
      logs.push({ type, text, location: msg.location() });
    }
  });
  page.on('pageerror', (err) => logs.push({ type: 'pageerror', text: err.message }));
  page.on('response', async (res) => {
    const req = res.request();
    const url = req.url();
    if (/probe=1|\.png|\.jpg|\.svg|\.ico|\.woff/i.test(url)) return;
    const status = res.status();
    if (status >= 400) {
      let body = '';
      try { body = await res.text(); } catch {}
      network.push({ url, method: req.method(), status, statusText: res.statusText(), body: body.slice(0, 1000) });
    }
  });

  log('Navigating to /agents');
  await page.goto(`${BASE_URL}/agents`, { waitUntil: 'domcontentloaded', timeout: 15000 });
  await page.waitForTimeout(3500);

  // Ensure grid/card view so duplicate buttons are visible.
  const gridBtn = page.locator('button').filter({ hasText: /^grid_view$/ }).first();
  if (await gridBtn.count() > 0) {
    await gridBtn.click();
    await page.waitForTimeout(800);
  }

  // Capture screenshot for debugging.
  const shotPath = join(OUT_DIR, 'agent-duplicate-before.png');
  await page.screenshot({ path: shotPath, fullPage: true }).catch(() => {});

  // Try card view duplicate button first.
  let dupBtn = page.locator('.agent-card button:has-text("复制")').first();
  let count = await dupBtn.count();
  log(`card duplicate buttons: ${count}`);

  if (count === 0) {
    // Fallback to list/table view icon button.
    dupBtn = page.locator('button i:has-text("content_copy"), button .q-icon:has-text("content_copy")').locator('..').first();
    count = await dupBtn.count();
    log(`icon duplicate buttons: ${count}`);
  }

  if (count === 0) {
    // Try any button whose text contains 复制 (card view).
    dupBtn = page.locator('button').filter({ hasText: /^复制$/ }).first();
    count = await dupBtn.count();
    log(`text duplicate buttons: ${count}`);
  }

  // If no duplicate button, create a minimal agent via API first.
  if (count === 0) {
    log('No duplicate button; creating a test agent via API');
    const testName = 'QA Dup Test ' + Date.now();
    const testKey = 'qa-dup-test-' + Date.now();
    const createRes = await page.evaluate(async ({ baseUrl, name, key }) => {
      const res = await fetch(`${baseUrl}/v1/agents`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          agentKey: key,
          displayName: name,
          provider: 'deepseek',
          model: 'deepseek-v4-flash',
          agentKind: 'standard',
          agentVariant: key,
        }),
      });
      const text = await res.text();
      return { status: res.status, text };
    }, { baseUrl: BASE_URL, name: testName, key: testKey });
    log(`API create status: ${createRes.status}`);
    if (createRes.status >= 400) {
      log(`API create body: ${createRes.text.slice(0, 500)}`);
      log('FAIL: API create failed');
      await browser.close();
      process.exit(1);
    }

    // Refresh /agents list.
    await page.goto(`${BASE_URL}/agents`, { waitUntil: 'domcontentloaded', timeout: 15000 });
    await page.waitForTimeout(2500);
    if (await gridBtn.count() > 0) {
      await gridBtn.click();
      await page.waitForTimeout(800);
    }

    // Search for the new agent card.
    dupBtn = page.locator(`.agent-card:has-text("${testName}") button:has-text("复制")`).first();
    count = await dupBtn.count();
    log(`created agent duplicate buttons: ${count}`);
  }

  if (count === 0) {
    // Debug: list all button texts.
    const btnTexts = await page.locator('button').evaluateAll((btns) =>
      btns.map((b) => ({ text: b.textContent?.trim().slice(0, 40), aria: b.getAttribute('aria-label'), class: b.className?.slice(0, 60) }))
    );
    log('All buttons:');
    for (const b of btnTexts.slice(0, 40)) log(`  ${JSON.stringify(b)}`);
    log('FAIL: no duplicate button found');
    await browser.close();
    process.exit(1);
  }

  log('Clicking duplicate button');
  await dupBtn.click();
  await page.waitForTimeout(2000);

  // Look for success notification.
  const notify = page.locator('.q-notification:has-text("Agent 已复制"), .q-notification:has-text("复制成功")').first();
  const success = await notify.count() > 0;

  log(`Duplicate result: success=${success}`);
  log(`Logs: ${logs.length}, Network errors: ${network.length}`);
  for (const n of network) log(`  NETWORK ${n.method} ${n.status} ${n.url} body=${n.body.slice(0, 300)}`);
  for (const l of logs) log(`  LOG [${l.type}] ${l.text}`);

  await browser.close();

  if (!success || network.length > 0 || logs.length > 0) {
    process.exit(1);
  }
})();
