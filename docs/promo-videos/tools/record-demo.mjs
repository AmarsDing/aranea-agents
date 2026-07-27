#!/usr/bin/env node
/**
 * record-demo.mjs · EP1 产品演示录屏
 *
 * 用 Playwright 无头 Chromium 打开 Aranea-Agents 聊天页，
 * 新建会话 → 输入演示需求 → 发送 → 录制整个精灵编排过程，
 * 输出 1920x1080 WebM 录屏 + 过程监控截图。
 *
 * 用法：
 *   node record-demo.mjs [--prompt "帮我调研新能源行业并生成一份投资报告"] [--out-dir ../ep1/recordings]
 *
 * 完成检测（宽松策略，宁可多录也不少录）：
 *   1. 先等到「团队活动出现」（页面文本命中团队/成员关键词）
 *   2. 之后页面文本静默 45s 且距发送超过 120s → 认为完成
 *   3. 硬上限 12 分钟，到点强制收尾（素材照样可用）
 */

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright-core';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function parseArgs(argv) {
  const args = {
    prompt: '帮我调研新能源行业并生成一份投资报告',
    outDir: path.resolve(__dirname, '../ep1/recordings'),
    baseUrl: 'http://localhost:9001/#/chat',
  };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--prompt') args.prompt = argv[++i];
    else if (a === '--out-dir') args.outDir = path.resolve(argv[++i]);
    else if (a === '--base-url') args.baseUrl = argv[++i];
  }
  return args;
}

const TEAM_HINT = /团队|研究员|分析师|撰稿人|运行中|进行中|已完成/;
const QUIET_MS = 60_000;
const MIN_RUN_MS = 180_000;
const HARD_CAP_MS = 15 * 60_000;
const POLL_MS = 5_000;
const MONITOR_EVERY_MS = 20_000;

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function main() {
  const args = parseArgs(process.argv);
  const monitorDir = path.join(args.outDir, 'monitor');
  fs.mkdirSync(args.outDir, { recursive: true });
  fs.mkdirSync(monitorDir, { recursive: true });

  console.log(`[record] out=${args.outDir}`);
  console.log(`[record] prompt=${args.prompt}`);

  // 复用本机已下载的 chromium_headless_shell-1223（与 playwright-core 期望版本不同，
  // 通过 executablePath 显式指定，避免重复下载浏览器）
  const localAppData = process.env.LOCALAPPDATA;
  const headlessShell = path.join(
    localAppData,
    'ms-playwright/chromium_headless_shell-1223/chrome-headless-shell-win64/chrome-headless-shell.exe',
  );
  const launchOpts = { headless: true };
  if (fs.existsSync(headlessShell)) launchOpts.executablePath = headlessShell;
  const browser = await chromium.launch(launchOpts);
  const context = await browser.newContext({
    viewport: { width: 1920, height: 1080 },
    recordVideo: { dir: args.outDir, size: { width: 1920, height: 1080 } },
    locale: 'zh-CN',
  });
  const page = await context.newPage();

  // 1. 打开聊天页
  await page.goto(args.baseUrl, { waitUntil: 'domcontentloaded', timeout: 60_000 });
  await page.waitForSelector('text=新会话', { timeout: 60_000 });
  await sleep(3000);
  await page.screenshot({ path: path.join(monitorDir, '01-loaded.png') });
  console.log('[record] page loaded');

  // 2. 新建会话
  await page.getByRole('button', { name: '新会话' }).first().click();
  await sleep(1500);
  await page.screenshot({ path: path.join(monitorDir, '02-new-session.png') });
  console.log('[record] new session created');

  // 3. 找到底部输入框并输入需求（逐字输入，录屏更自然）
  const composer = page.locator('textarea:visible').last();
  await composer.click();
  await composer.pressSequentially(args.prompt, { delay: 90 });
  await sleep(600);
  await page.screenshot({ path: path.join(monitorDir, '03-typed.png') });
  console.log('[record] prompt typed');

  // 4. 发送（Enter）
  await composer.press('Enter');
  const sentAt = Date.now();
  console.log('[record] sent, orchestration running...');

  // 5. 轮询等待编排完成
  let lastText = '';
  let lastChange = Date.now();
  let lastMonitor = 0;
  let sawTeam = false;
  let shotIdx = 10;

  while (true) {
    await sleep(POLL_MS);
    const now = Date.now();
    let text = '';
    try {
      text = await page.evaluate(() => document.body.innerText);
    } catch {
      continue;
    }
    if (text !== lastText) {
      lastText = text;
      lastChange = now;
    }
    if (!sawTeam && TEAM_HINT.test(text)) {
      sawTeam = true;
      console.log('[record] team activity detected');
    }
    if (now - lastMonitor > MONITOR_EVERY_MS) {
      lastMonitor = now;
      const p = path.join(monitorDir, `${String(shotIdx++).padStart(2, '0')}-run-${Math.round((now - sentAt) / 1000)}s.png`);
      await page.screenshot({ path: p }).catch(() => {});
      console.log(`[record] t=${Math.round((now - sentAt) / 1000)}s quiet=${Math.round((now - lastChange) / 1000)}s`);
    }
    const quietDone = sawTeam && now - lastChange > QUIET_MS && now - sentAt > MIN_RUN_MS;
    const capped = now - sentAt > HARD_CAP_MS;
    if (quietDone || capped) {
      console.log(`[record] finish: ${quietDone ? 'quiet-complete' : 'hard-cap'}`);
      break;
    }
  }

  // 6. 收尾：多录 5s 最终状态
  await sleep(5000);
  await page.screenshot({ path: path.join(monitorDir, '99-final.png') });

  // 关闭 context 才会落盘视频
  await context.close();
  await browser.close();

  // 重命名最新 webm
  const webms = fs.readdirSync(args.outDir).filter((f) => f.endsWith('.webm'));
  if (webms.length) {
    const newest = webms
      .map((f) => ({ f, t: fs.statSync(path.join(args.outDir, f)).mtimeMs }))
      .sort((a, b) => b.t - a.t)[0].f;
    const target = path.join(args.outDir, 'ep1-demo-raw.webm');
    fs.renameSync(path.join(args.outDir, newest), target);
    console.log(`[record] video: ${target}`);
  } else {
    console.error('[record] WARN: no webm produced');
  }
  console.log(`[record] total: ${Math.round((Date.now() - sentAt) / 1000)}s`);
}

main().catch((err) => {
  console.error(`record-demo 失败：${err.message}`);
  console.error(err.stack);
  process.exit(1);
});
