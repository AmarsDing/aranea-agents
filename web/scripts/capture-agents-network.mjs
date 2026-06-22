import { chromium } from 'playwright';
import { writeFileSync, mkdirSync } from 'fs';
import { join } from 'path';

const BASE_URL = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:9001';
const OUT_DIR = join(process.cwd(), '..', '.dogfood-output');
mkdirSync(OUT_DIR, { recursive: true });

const IGNORED_URL = /probe=1|\.png|\.jpg|\.svg|\.ico|\.woff/i;

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
        headers: await res.allHeaders(),
      });
    }
  });
  const consoleLogs = [];
  page.on('console', (msg) => {
    const type = msg.type();
    const text = msg.text();
    if (type === 'error' || type === 'warning') consoleLogs.push({ type, text });
  });
  page.on('pageerror', (err) => consoleLogs.push({ type: 'pageerror', text: err.message }));

  await page.goto(`${BASE_URL}/agents`, { waitUntil: 'domcontentloaded', timeout: 15000 });
  await page.waitForTimeout(8000);
  await page.screenshot({ path: join(OUT_DIR, 'agents-network.png'), fullPage: true });
  await browser.close();

  const report = { network, consoleLogs };
  const reportPath = join(OUT_DIR, 'agents-network-report.json');
  writeFileSync(reportPath, JSON.stringify(report, null, 2));
  console.log(`Report: ${reportPath}`);
  console.log(`4xx/5xx responses: ${network.length}`);
  for (const n of network) {
    console.log(`${n.method} ${n.status} ${n.url}\n${n.body.slice(0, 500)}\n---`);
  }
})();
