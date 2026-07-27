#!/usr/bin/env node
/**
 * render-animation.mjs · 把 ep1/animation.html 录制成 1920x1080 WebM
 *
 * 动画页是时间轴驱动的（TOTAL=117.672s），用 Playwright 无头 Chromium
 * 打开 ?record=1 实时播放整段并录屏，等待 window.__done 后收尾。
 *
 * 用法：
 *   node render-animation.mjs [--url http://localhost:8899/ep1/animation.html] [--out ../ep1/recordings/ep1-animation-raw.webm]
 *
 * 前置：先启动 serve.mjs（默认端口 8899）。
 */

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright-core';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function parseArgs(argv) {
  const args = {
    url: 'http://localhost:8899/ep1/animation.html',
    out: path.resolve(__dirname, '../ep1/recordings/ep1-animation-raw.webm'),
    scale: 1, // 2 = 3840x2160 超采样渲染，缩回 1080p 可显著提升锐度
  };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--url') args.url = argv[++i];
    else if (a === '--out') args.out = path.resolve(argv[++i]);
    else if (a === '--scale') args.scale = parseFloat(argv[++i]);
  }
  return args;
}

const TOTAL_S = 117.672;
const HARD_CAP_MS = (TOTAL_S + 30) * 1000;

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

  // 轮询 window.__done
  while (true) {
    await sleep(3000);
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

  await sleep(500); // 让最后一帧落盘
  await context.close();
  await browser.close();

  // 重命名最新 webm
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
  console.error(`render-animation 失败：${err.message}`);
  console.error(err.stack);
  process.exit(1);
});
