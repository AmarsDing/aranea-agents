import { chromium } from 'playwright';
import { writeFileSync, mkdirSync } from 'fs';
import { join } from 'path';

const BASE_URL = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:9001';
const ROUTE = process.env.ROUTE || '/';
const OUT_DIR = join(process.cwd(), '..', '.dogfood-output');
mkdirSync(OUT_DIR, { recursive: true });

const IGNORED_URL = /probe=1|\.png|\.jpg|\.svg|\.ico|\.woff/i;
const IGNORED_CONSOLE = /WebSocket connection to .*probe=1.* failed: WebSocket is closed before the connection is established/i;

(async () => {
  const browser = await chromium.launch({
    headless: true,
    executablePath: process.env.CHROME_EXE || 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const page = await context.newPage();

  const network = [];
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
      network.push({
        url,
        method: req.method(),
        status,
        statusText: res.statusText(),
        body: body.slice(0, 2000),
      });
    }
  });

  const logs = [];
  page.on('console', (msg) => {
    const type = msg.type();
    const text = msg.text();
    if (IGNORED_CONSOLE.test(text)) return;
    if (type === 'error' || type === 'warning') {
      logs.push({ type, text });
    }
  });
  page.on('pageerror', (err) => logs.push({ type: 'pageerror', text: err.message }));

  const url = `${BASE_URL}${ROUTE}`;
  const loadOk = await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 15000 })
    .then(() => true)
    .catch((e) => {
      logs.push({ type: 'loaderror', text: e.message });
      return false;
    });
  await page.waitForTimeout(8000);

  const slug = ROUTE.replace(/\//g, '_') || 'root';
  const screenshotPath = join(OUT_DIR, `${slug}-check.png`);
  await page.screenshot({ path: screenshotPath, fullPage: true }).catch(() => {});

  await browser.close();

  const report = { route: ROUTE, url, loadOk, network, logs, screenshot: screenshotPath };
  const reportPath = join(OUT_DIR, `${slug}-check-report.json`);
  writeFileSync(reportPath, JSON.stringify(report, null, 2));
  console.log(`Route: ${ROUTE} loadOk=${loadOk} network=${network.length} logs=${logs.length}`);
  console.log(`Report: ${reportPath}`);
  for (const n of network) {
    console.log(`${n.method} ${n.status} ${n.url}`);
    console.log(n.body.slice(0, 300));
    console.log('---');
  }
  for (const l of logs) {
    console.log(`[${l.type}] ${l.text.slice(0, 300)}`);
  }
})();
