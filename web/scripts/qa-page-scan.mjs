import { chromium } from 'playwright';
import { writeFileSync, mkdirSync } from 'fs';
import { join } from 'path';

const BASE_URL = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:9001';
const OUT_DIR = join(process.cwd(), '..', '.dogfood-output');
const SHOT_DIR = join(OUT_DIR, 'screenshots');
mkdirSync(SHOT_DIR, { recursive: true });

const IGNORED_CONSOLE = /WebSocket connection to .*probe=1.* failed: WebSocket is closed before the connection is established/i;

const ROUTES = [
  '/overview',
  '/usage/events',
  '/chat',
  '/sessions',
  '/memory',
  '/agents',
  '/settings/organization',
  '/team',
  '/graphs',
  '/models',
  '/channels',
  '/mcp-servers',
  '/skills',
  '/skills/runs',
  '/skills/evolution-suggestions',
  '/skills/experience-reports',
  '/plugins',
  '/plugins/runs',
  '/hooks',
  '/hooks/deliveries',
  '/webhooks',
  '/knowledge',
  '/artifacts',
  '/evaluation',
  '/a2a',
  '/tools',
  '/tools/runs',
  '/tools/audits',
  '/cron',
  '/monitor/logs',
  '/observability',
  '/shop',
  '/settings',
];

(async () => {
  const browser = await chromium.launch({
    headless: true,
    executablePath: process.env.CHROME_EXE || 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  const results = [];

  for (const path of ROUTES) {
    console.log(`Scanning ${path}...`);
    const page = await context.newPage();
    const logs = [];
    page.on('console', (msg) => {
      const type = msg.type();
      const text = msg.text();
      if (IGNORED_CONSOLE.test(text)) return;
      if (type === 'error' || type === 'warning') {
        logs.push({ type, text, location: msg.location() });
      }
    });
    page.on('pageerror', (err) => logs.push({ type: 'pageerror', text: err.message }));

    const url = `${BASE_URL}${path}`;
    const slug = path.replace(/\//g, '_') || 'root';
    const started = Date.now();
    let loadOk = false;
    let error = null;
    try {
      await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 15000 });
      await page.waitForTimeout(8500);
      loadOk = true;
    } catch (e) {
      error = e.message;
    }
    const screenshotPath = join(SHOT_DIR, `${slug}.png`);
    try {
      await page.screenshot({ path: screenshotPath, fullPage: true });
    } catch (shotErr) {
      logs.push({ type: 'screenshot-error', text: shotErr.message });
    }
    results.push({
      path,
      url,
      loadOk,
      loadMs: Date.now() - started,
      error,
      logs,
      screenshot: screenshotPath,
    });
    await page.close();
  }

  await browser.close();
  const reportPath = join(OUT_DIR, 'scan-report.json');
  writeFileSync(reportPath, JSON.stringify(results, null, 2));
  console.log(`Scanned ${ROUTES.length} routes. Report: ${reportPath}`);
  const failed = results.filter((r) => !r.loadOk || r.logs.length > 0);
  console.log(`Routes with load errors or console warnings/errors: ${failed.length}`);
  for (const r of failed) {
    console.log(`- ${r.path}: loadOk=${r.loadOk} logs=${r.logs.length}${r.error ? ' error=' + r.error : ''}`);
  }
})();
