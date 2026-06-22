import { chromium } from 'playwright';
import { writeFileSync, mkdirSync, appendFileSync } from 'fs';
import { join } from 'path';

const BASE_URL = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:9001';
const OUT_DIR = join(process.cwd(), '..', '.dogfood-output');
const SHOT_DIR = join(OUT_DIR, 'target-screenshots');
mkdirSync(SHOT_DIR, { recursive: true });
const RUN_LOG = join(OUT_DIR, 'target-check.log');
writeFileSync(RUN_LOG, '');

function log(msg) {
  console.log(msg);
  appendFileSync(RUN_LOG, msg + '\n');
}

const ROUTES = ['/agents', '/channels'];
const IGNORED_CONSOLE = /WebSocket connection to .*probe=1.* failed: WebSocket is closed before the connection is established/i;
const IGNORED_URL = /probe=1|\.png|\.jpg|\.svg|\.ico|\.woff/i;

function slug(path) {
  return path.replace(/\//g, '_') || 'root';
}

(async () => {
  const browser = await chromium.launch({
    headless: true,
    executablePath: process.env.CHROME_EXE || 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const results = [];

  for (const path of ROUTES) {
    log(`\n=== Target check ${path} ===`);
    const page = await context.newPage();
    const logs = [];
    const network = [];

    page.on('console', (msg) => {
      const type = msg.type();
      const text = msg.text();
      if (IGNORED_CONSOLE.test(text)) return;
      if (type === 'error' || type === 'warning') {
        logs.push({ type, text, location: msg.location() });
      }
    });
    page.on('pageerror', (err) => logs.push({ type: 'pageerror', text: err.message }));
    page.on('response', async (res) => {
      const req = res.request();
      const url = req.url();
      if (IGNORED_URL.test(url)) return;
      const status = res.status();
      if (status >= 400) {
        let body = '';
        try {
          body = await res.text();
        } catch {}
        network.push({ url, method: req.method(), status, statusText: res.statusText(), body: body.slice(0, 2000) });
      }
    });
    page.on('dialog', async (dialog) => {
      log(`  dialog: ${dialog.type()} - ${dialog.message()}`);
      await dialog.dismiss().catch(() => {});
    });

    const url = `${BASE_URL}${path}`;
    let loadOk = false;
    let error = null;
    try {
      await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 15000 });
      await page.waitForTimeout(5000);
      loadOk = true;
    } catch (e) {
      error = e.message;
    }

    const screenshotPath = join(SHOT_DIR, `${slug(path)}_check.png`);
    await page.screenshot({ path: screenshotPath, fullPage: true }).catch(() => {});

    results.push({ path, url, loadOk, error, logs: [...logs], network: [...network], screenshot: screenshotPath });
    log(`  done: loadOk=${loadOk}, logs=${logs.length}, network=${network.length}${error ? ' error=' + error : ''}`);
    for (const n of network) {
      log(`  NETWORK ${n.method} ${n.status} ${n.url}`);
      log(`    ${n.body.slice(0, 200).replace(/\n/g, ' ')}`);
    }
    for (const l of logs) {
      log(`  [${l.type}] ${l.text.slice(0, 200)}`);
    }
    await page.close();
  }

  await browser.close();
  const reportPath = join(OUT_DIR, 'target-check-report.json');
  writeFileSync(reportPath, JSON.stringify(results, null, 2));
  log(`\nTarget check report: ${reportPath}`);
})();
