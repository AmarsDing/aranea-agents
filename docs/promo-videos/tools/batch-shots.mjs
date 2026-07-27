#!/usr/bin/env node
/**
 * batch-shots.mjs · EP3 宣传视频 · 真实程序截图批量采集
 *
 * 用 Playwright 无头 Chromium 以 1920x1080 视口逐页访问前端并截图，
 * 供 animation.html 以「真实截图 + 动画标注」方式嵌入。
 *
 * 用法：
 *   node batch-shots.mjs [--base http://localhost:9001] [--out-dir ../ep3/shots]
 *
 * 前置：后端 admin (:8000) 与前端 pnpm dev (:9001) 均在运行。
 */

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright-core';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function parseArgs(argv) {
  const args = {
    base: 'http://localhost:9001',
    outDir: path.resolve(__dirname, '../ep3/shots'),
    only: null,
  };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--base') args.base = argv[++i];
    else if (a === '--out-dir') args.outDir = path.resolve(argv[++i]);
    else if (a === '--only') args.only = argv[++i].split(',');
  }
  return args;
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// [文件名, hash 路由, 载入后额外等待 ms]
const SHOTS = [
  ['overview',      '/#/overview',      4000],
  ['memory',        '/#/memory',        4500],
  ['observability', '/#/observability', 4500],
  ['usage-events',  '/#/usage/events',  4500],
  ['skills',        '/#/skills',        4000],
  ['evaluation',    '/#/evaluation',    4500],
  ['channels',      '/#/channels',      4500],
  ['shop',          '/#/shop',          4000],
  ['teams',         '/#/team',          4500],
  ['graphs',        '/#/graphs',        4000],
  ['organization',  '/#/settings/organization', 5000],
  ['agents',        '/#/agents',        4500],
  ['knowledge',     '/#/knowledge',     4000],
  ['cron',          '/#/cron',          4000],
  ['plugins',       '/#/plugins',       4000],
];

async function main() {
  const args = parseArgs(process.argv);
  fs.mkdirSync(args.outDir, { recursive: true });

  const localAppData = process.env.LOCALAPPDATA;
  const headlessShell = path.join(
    localAppData,
    'ms-playwright/chromium_headless_shell-1223/chrome-headless-shell-win64/chrome-headless-shell.exe',
  );
  const launchOpts = { headless: true };
  if (fs.existsSync(headlessShell)) launchOpts.executablePath = headlessShell;
  const browser = await chromium.launch(launchOpts);
  const context = await browser.newContext({ viewport: { width: 1920, height: 1080 } });
  const page = await context.newPage();

  const shots = args.only ? SHOTS.filter(([n]) => args.only.includes(n)) : SHOTS;
  for (const [name, route, wait] of shots) {
    const url = `${args.base}${route}`;
    const out = path.join(args.outDir, `${name}.png`);
    try {
      await page.goto(url, { waitUntil: 'load', timeout: 45_000 });
      await sleep(wait);
      await page.screenshot({ path: out, type: 'png' });
      console.log(`[shot] ${name} <- ${url}`);
    } catch (err) {
      console.log(`[shot] ${name} FAILED: ${err.message}`);
    }
  }

  // Agent 设置页：从 agents 页抓第一个「设置」链接
  try {
    await page.goto(`${args.base}/#/agents`, { waitUntil: 'load', timeout: 45_000 });
    await sleep(4000);
    const href = await page.evaluate(() => {
      const a = document.querySelector('a[href*="/settings"], a[href*="agents/"]');
      return a ? a.getAttribute('href') : null;
    });
    if (href) {
      const url = href.startsWith('http') ? href : `${args.base}/${href.replace(/^#?\/?/, '#/')}`;
      await page.goto(url, { waitUntil: 'load', timeout: 45_000 });
      await sleep(5000);
      await page.screenshot({ path: path.join(args.outDir, 'agent-settings.png'), type: 'png' });
      console.log(`[shot] agent-settings <- ${url}`);
    } else {
      console.log('[shot] agent-settings: 未找到设置入口链接，跳过');
    }
  } catch (err) {
    console.log(`[shot] agent-settings FAILED: ${err.message}`);
  }

  await context.close();
  await browser.close();
  console.log(`[done] 截图输出目录: ${args.outDir}`);
}

main().catch((err) => {
  console.error(`batch-shots 失败：${err.message}`);
  process.exit(1);
});
