#!/usr/bin/env node
/**
 * render-ep3.mjs · EP3 专用渲染器（总时长 307.272s）
 *
 * 用法：
 *   node render-ep3.mjs [--scale 1]
 */

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright-core';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function parseArgs(argv) {
  const args = {
    url: 'http://localhost:8899/ep3/animation.html',
    out: path.resolve(__dirname, '../ep3/recordings/ep3-animation-raw.webm'),
    scale: 1,
  };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--url') args.url = argv[++i];
    else if (a === '--out') args.out = path.resolve(argv[++i]);
    else if (a === '--scale') args.scale = parseFloat(argv[++i]);
  }
  return args;
}

const TOTAL_S = 307.272;
const HARD_CAP_MS = (TOTAL_S + 60) * 1000;

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function main() {
  const args = parseArgs(process.argv);
  const outDir = path.dirname(args.out);
  fs.mkdirSync(outDir, { recursive: true });

  const W = Math.round(1920 * args.scale);
  const H = Math.round(1080 * args.scale);
  console.log(`[render] url=${args.url}?record=1&scale=${args.scale} (${W}x${H})`);
  console.log(`[render] out=${args.out}`);

  const localAppData = process.env.LOCALAPPDATA;
  const headlessShell = path.join(
    localAppData,
    'ms-playwright/chromium_headless_shell-1223/chrome-headless-shell-win64/chrome-headless-shell.exe',
  );
  const launchOpts = { headless: true };
  if (fs.existsSync(headlessShell)) launchOpts.executablePath = headlessShell;
  const browser = await chromium.launch(launchOpts);
  const context = await browser.newContext({
    viewport: { width: W, height: H },
    recordVideo: { dir: outDir, size: { width: W, height: H } },
  });
  const page = await context.newPage();
  page.on('console', (m) => {
    if (m.type() === 'error') console.log(`[page-err] ${m.text()}`);
  });
  page.on('pageerror', (e) => console.log(`[page-err] ${e.message}`));

  await page.goto(`${args.url}?record=1&scale=${args.scale}`, { waitUntil: 'load', timeout: 60_000 });
  const t0 = Date.now();
  console.log('[render] playing...');

  while (true) {
    await sleep(5000);
    const done = await page.evaluate(() => window.__done === true).catch(() => false);
    if (done) {
      console.log(`[render] done at ${Math.round((Date.now() - t0) / 1000)}s`);
      break;
    }
    if (Date.now() - t0 > HARD_CAP_MS) {
      console.log('[render] WARN: hard cap reached, stopping anyway');
      break;
    }
  }

  await sleep(800);
  await context.close();
  await browser.close();

  const webms = fs.readdirSync(outDir).filter((f) => f.endsWith('.webm') && f !== path.basename(args.out));
  if (webms.length) {
    const newest = webms
      .map((f) => ({ f, t: fs.statSync(path.join(outDir, f)).mtimeMs }))
      .sort((a, b) => b.t - a.t)[0].f;
    fs.renameSync(path.join(outDir, newest), args.out);
    console.log(`[render] video: ${args.out}`);
  } else {
    console.error('[render] WARN: no webm produced');
  }
}

main().catch((err) => {
  console.error(`render-ep3 失败：${err.message}`);
  console.error(err.stack);
  process.exit(1);
});
