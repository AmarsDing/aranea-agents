import { chromium } from 'playwright';
import { writeFileSync, mkdirSync, appendFileSync } from 'fs';
import { join } from 'path';

const BASE_URL = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:9001';
const OUT_DIR = join(process.cwd(), '..', '.dogfood-output');
const SHOT_DIR = join(OUT_DIR, 'interactive-screenshots');
mkdirSync(SHOT_DIR, { recursive: true });
const RUN_LOG = join(OUT_DIR, 'interactive-run.log');
writeFileSync(RUN_LOG, '');

function log(msg) {
  console.log(msg);
  appendFileSync(RUN_LOG, msg + '\n');
}

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

const IGNORED_TEXT = /logout|退出|登出|github|twitter|x\.com|docs|帮助文档|privacy|terms|license|重新检测|brightness_auto|上传|导入|upload|import|cloud_upload|attach_file|^D$|^menu$|^chevron_right$|^chevron_left$|^notifications_none$/i;
const IGNORED_CONSOLE = /WebSocket connection to .*probe=1.* failed: WebSocket is closed before the connection is established/i;
const IGNORED_NETWORK_URL = /probe=1|\.png|\.jpg|\.svg|\.ico|\.woff/i;
const MAX_CLICKS_PER_PAGE = 30;
const START_INDEX = Number(process.env.QA_START_INDEX || 0);

function slug(path) {
  return path.replace(/\//g, '_') || 'root';
}

async function waitForBackend(maxAttempts = 30) {
  for (let i = 0; i < maxAttempts; i++) {
    try {
      const res = await fetch(`${BASE_URL}/healthz`, { signal: AbortSignal.timeout(3000) });
      if (res.ok) return true;
    } catch {}
    await new Promise((r) => setTimeout(r, 1000));
  }
  return false;
}

(async () => {
  log('Waiting for backend health...');
  const backendOk = await waitForBackend();
  if (!backendOk) {
    log('Backend not healthy; aborting.');
    process.exit(1);
  }
  log('Backend healthy.');

  const browser = await chromium.launch({
    headless: true,
    executablePath: process.env.CHROME_EXE || 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  });
  const results = [];

  for (const path of ROUTES.slice(START_INDEX)) {
    log(`\n=== Testing ${path} ===`);
    const context = await browser.newContext({ viewport: { width: 1440, height: 900 } });
    let page;
    try {
      page = await context.newPage();
    } catch (e) {
      log(`  failed to create page: ${e.message}`);
      results.push({ path, url: `${BASE_URL}${path}`, loadOk: false, error: e.message, clickedCount: 0, clicked: [], logs: [], network: [] });
      try { await context.close(); } catch {}
      continue;
    }
    const logs = [];
    const network = [];
    const clicked = [];
    try {
      page.on('response', async (res) => {
      const req = res.request();
      const url = req.url();
      if (IGNORED_NETWORK_URL.test(url)) return;
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
          body: body.slice(0, 1000),
        });
      }
    });
    page.on('console', (msg) => {
      const type = msg.type();
      const text = msg.text();
      if (IGNORED_CONSOLE.test(text)) return;
      if (type === 'error' || type === 'warning') {
        logs.push({ type, text, location: msg.location() });
      }
    });
    page.on('pageerror', (err) => logs.push({ type: 'pageerror', text: err.message }));
    page.on('dialog', async (dialog) => {
      log(`  dialog: ${dialog.type()} - ${dialog.message()}`);
      await dialog.dismiss().catch(() => {});
    });

    const url = `${BASE_URL}${path}`;
    let loadOk = false;
    let error = null;
    try {
      await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 15000 });
      await page.waitForTimeout(2000);
      loadOk = true;
    } catch (e) {
      error = e.message;
    }

    async function closeActiveDialogs() {
      let closed = false;
      for (let i = 0; i < 4; i++) {
        const backdrops = await page.locator('.q-dialog__backdrop').all();
        const menus = await page.locator('.q-menu, .q-popup-proxy').count();
        if (backdrops.length === 0 && menus === 0) break;
        if (backdrops.length > 0) {
          for (const bd of backdrops) {
            await bd.click({ timeout: 1000 }).catch(() => {});
            await page.waitForTimeout(200);
          }
          const closeBtnSelectors = [
            '.q-dialog .q-btn[icon="close"]',
            '.q-dialog [v-close-popup]',
            '.q-dialog button',
            '.q-dialog [role="button"]',
          ];
          for (const selector of closeBtnSelectors) {
            const closeBtns = await page.locator(selector).all();
            for (const btn of closeBtns) {
              const text = (await btn.textContent().catch(() => '')).trim();
              const icon = (await btn.getAttribute('icon').catch(() => '')) || '';
              if (
                icon === 'close' ||
                /^(取消|关闭|Close|Cancel|Dismiss|OK|确定|确认)$/i.test(text)
              ) {
                await btn.click({ timeout: 1000 }).catch(() => {});
                await page.waitForTimeout(200);
              }
            }
          }
        }
        await page.keyboard.press('Escape').catch(() => {});
        await page.waitForTimeout(400);
        closed = true;
      }
      return closed;
    }

    if (loadOk) {
      await closeActiveDialogs();
      await page.waitForTimeout(500);

      let count = 0;
      const clickedKeys = new Set();
      while (count < MAX_CLICKS_PER_PAGE) {
        const closed = await closeActiveDialogs();
        if (closed) await page.waitForTimeout(500);

        const interactive = await page.locator('button:not([disabled]), a, [role="button"], [role="link"], .q-item[clickable], .q-btn, .q-item--clickable').all();
        let picked = null;
        for (const el of interactive) {
          try {
            const visible = await el.isVisible().catch(() => false);
            if (!visible) continue;
            const enabled = await el.isEnabled().catch(() => true);
            if (!enabled) continue;
            const text = (await el.textContent().catch(() => '')).trim().slice(0, 60);
            const href = await el.getAttribute('href').catch(() => null);
            if (IGNORED_TEXT.test(text)) continue;
            if (href && (href.startsWith('http') || href.startsWith('mailto'))) continue;
            const tag = await el.evaluate((n) => n.tagName.toLowerCase());
            const box = await el.boundingBox().catch(() => null);
            if (!box || box.width < 4 || box.height < 4) continue;
            const inDrawer = await el.evaluate((n) => !!n.closest('.q-drawer')).catch(() => false);
            if (inDrawer) continue;
            const key = `${tag}|${text}|${href || ''}`;
            if (clickedKeys.has(key)) continue;
            picked = { el, tag, text, href, key };
            break;
          } catch (e) {
            continue;
          }
        }
        if (!picked) break;

        count++;
        clickedKeys.add(picked.key);
        log(`  click ${count}: ${picked.tag} "${picked.text}"`);
        const beforeUrl = page.url();
        clicked.push({ tag: picked.tag, text: picked.text, href: picked.href });
        try {
          await picked.el.click({ timeout: 3000 });
        } catch (e) {
          log(`    click failed: ${e.message}`);
        }
        await page.waitForTimeout(800);
        const afterUrl = page.url();
        if (afterUrl !== beforeUrl) {
          log(`    navigated to ${afterUrl}`);
          await page.goBack({ waitUntil: 'domcontentloaded', timeout: 10000 }).catch(() => {});
          await page.waitForTimeout(1000);
        }
        const shot = join(SHOT_DIR, `${slug(path)}_click_${count}.png`);
        await page.screenshot({ path: shot, fullPage: false }).catch(() => {});
      }
    }

    const screenshotPath = join(SHOT_DIR, `${slug(path)}_final.png`);
    try {
      await page.screenshot({ path: screenshotPath, fullPage: true });
    } catch (shotErr) {
      logs.push({ type: 'screenshot-error', text: shotErr.message });
    }

    results.push({
      path,
      url,
      loadOk,
      error,
      clickedCount: clicked.length,
      clicked,
      logs: [...logs],
      network: [...network],
    });

      log(`  done: loadOk=${loadOk}, clicks=${clicked.length}, logs=${logs.length}, network=${network.length}`);
    } catch (routeErr) {
      log(`  route error: ${routeErr.message}`);
      results.push({
        path,
        url: `${BASE_URL}${path}`,
        loadOk: false,
        error: routeErr.message,
        clickedCount: clicked.length,
        clicked,
        logs: [...logs],
        network: [...network],
      });
    }

    try {
      await page.close();
    } catch {}
    try {
      await context.close();
    } catch {}
  }

  try {
    await browser.close();
  } catch {}
  const reportPath = join(OUT_DIR, 'interactive-report.json');
  writeFileSync(reportPath, JSON.stringify(results, null, 2));
  log(`\nInteractive test report: ${reportPath}`);
  const failed = results.filter((r) => !r.loadOk || r.logs.length > 0 || r.network.length > 0);
  log(`Routes with errors/warnings/network-errors: ${failed.length}`);
  for (const r of failed) {
    log(
      `- ${r.path}: loadOk=${r.loadOk} clicks=${r.clickedCount} logs=${r.logs.length} network=${r.network.length}${r.error ? ' error=' + r.error : ''}`,
    );
    for (const n of r.network) {
      log(`    NETWORK ${n.method} ${n.status} ${n.url}`);
    }
  }
})();
