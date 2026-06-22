import { chromium } from 'playwright';
import { writeFileSync, mkdirSync, appendFileSync } from 'fs';
import { join } from 'path';

const BASE_URL = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:9001';
const OUT_DIR = join(process.cwd(), '..', '.dogfood-output');
const SHOT_DIR = join(OUT_DIR, 'channels-icons');
mkdirSync(SHOT_DIR, { recursive: true });
const RUN_LOG = join(OUT_DIR, 'channels-icons.log');
writeFileSync(RUN_LOG, '');

function log(msg) {
  console.log(msg);
  appendFileSync(RUN_LOG, msg + '\n');
}

const IGNORED_CONSOLE = /WebSocket connection to .*probe=1.* failed: WebSocket is closed before the connection is established/i;

(async () => {
  log('Launching browser...');
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
    if (IGNORED_CONSOLE.test(text)) return;
    if (type === 'error' || type === 'warning') {
      logs.push({ type, text });
    }
  });
  page.on('pageerror', (err) => logs.push({ type: 'pageerror', text: err.message }));
  page.on('response', async (res) => {
    const req = res.request();
    const url = req.url();
    const status = res.status();
    if (status >= 400 && (url.includes('/v1/avatar-assets/') || url.includes('/v1/channels'))) {
      let body = '';
      try {
        body = await res.text();
      } catch {}
      network.push({ url, method: req.method(), status, body: body.slice(0, 500) });
    }
  });

  try {
    log('Opening /channels...');
    await page.goto(`${BASE_URL}/channels`, { waitUntil: 'domcontentloaded', timeout: 15000 });
    await page.waitForTimeout(2000);

    log('Clicking add channel button...');
    const addBtn = page.locator('button').filter({ hasText: /添加|Add|add/ }).first();
    if (await addBtn.count() === 0) {
      log('Add button not found; trying icon add');
      await page.locator('button i:has-text("add")').first().click({ timeout: 3000 });
    } else {
      await addBtn.click({ timeout: 3000 });
    }
    await page.waitForTimeout(1500);

    log('Checking platform catalog cards...');
    const cards = await page.locator('.catalog-card').all();
    log(`Found ${cards.length} catalog cards`);

    const results = [];
    for (let i = 0; i < cards.length; i++) {
      const card = cards[i];
      const title = await card.locator('.catalog-card__title').textContent().catch(() => '');
      const group = await card.locator('.catalog-card__group').textContent().catch(() => '');
      const img = card.locator('img.resolved-avatar-img');
      const imgCount = await img.count();
      let src = '';
      let naturalWidth = 0;
      if (imgCount > 0) {
        src = await img.getAttribute('src') || '';
        naturalWidth = await img.evaluate((el) => (el.naturalWidth || 0));
      }
      const textFallback = await card.locator('span').textContent().catch(() => '');
      results.push({ index: i, title: title.trim(), group: group.trim(), imgCount, src: src.slice(0, 200), naturalWidth, textFallback: textFallback.trim().slice(0, 20) });
    }

    log('\nPlatform icons summary:');
    for (const r of results) {
      const ok = r.imgCount > 0 && r.naturalWidth > 0;
      log(`  [${ok ? 'OK' : 'FAIL'}] ${r.title} (${r.group}) img=${r.imgCount} width=${r.naturalWidth} src=${r.src || '-'} fallback=${r.textFallback || '-'}`);
    }

    await page.screenshot({ path: join(SHOT_DIR, 'catalog.png'), fullPage: true });

    const reportPath = join(OUT_DIR, 'channels-icons-report.json');
    writeFileSync(reportPath, JSON.stringify({ results, network, logs }, null, 2));
    log(`\nReport: ${reportPath}`);

    if (network.length) {
      log('\nNetwork errors:');
      for (const n of network) log(`  ${n.method} ${n.status} ${n.url}`);
    }
    if (logs.length) {
      log('\nConsole errors/warnings:');
      for (const l of logs) log(`  [${l.type}] ${l.text.slice(0, 200)}`);
    }

    const failed = results.filter((r) => r.imgCount === 0 || r.naturalWidth === 0);
    log(`\nFailed icons: ${failed.length}`);
    if (failed.length > 0) {
      log('FAIL');
      process.exitCode = 1;
    } else {
      log('OK');
    }
  } catch (e) {
    log(`ERROR: ${e.message}`);
    process.exitCode = 1;
  } finally {
    await browser.close();
  }
})();
