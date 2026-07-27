#!/usr/bin/env node
/**
 * record-demo-ep2.mjs · EP2 产品演示录屏（指挥官介入）
 *
 * 用 Playwright 无头 Chromium 打开 Aranea-Agents 聊天页：
 *   新建会话 → 发送演示需求 → 等团队并行执行 → 暂停一名成员
 *   → 注入新指令 → 等恢复执行 → 等报告完成。
 * 输出 1920x1080 WebM 录屏 + actions.json（动作时间戳，供剪辑对齐 cap）+ 监控截图。
 *
 * 用法：
 *   node record-demo-ep2.mjs [--prompt "帮我调研新能源行业并生成一份投资报告"]
 *                            [--inject "把调研重点，转向储能赛道"]
 *                            [--out-dir ../ep2/recordings]
 *
 * 完成检测（宽松策略，与 EP1 一致）：
 *   1. 先等到「团队活动出现」
 *   2. 页面文本静默 60s 且距发送超过 180s → 认为完成
 *   3. 硬上限 15 分钟，到点强制收尾
 */

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright-core';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

function parseArgs(argv) {
  const args = {
    prompt: '帮我调研新能源行业并生成一份投资报告',
    inject: '把调研重点，转向储能赛道',
    outDir: path.resolve(__dirname, '../ep2/recordings'),
    baseUrl: 'http://localhost:9001/#/chat',
  };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--prompt') args.prompt = argv[++i];
    else if (a === '--inject') args.inject = argv[++i];
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
  console.log(`[record] inject=${args.inject}`);

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

  const actions = []; // { t: 相对发送秒数, action: 名称, note }
  let sentAt = null;
  const mark = (action, note = '') => {
    const t = sentAt ? (Date.now() - sentAt) / 1000 : 0;
    actions.push({ t: Math.round(t * 10) / 10, action, note });
    console.log(`[action] t=${Math.round(t)}s ${action}${note ? ' · ' + note : ''}`);
  };

  // 1. 打开聊天页
  await page.goto(args.baseUrl, { waitUntil: 'domcontentloaded', timeout: 60_000 });
  await page.waitForSelector('text=新会话', { timeout: 60_000 });
  await sleep(3000);
  await page.screenshot({ path: path.join(monitorDir, '01-loaded.png') });
  console.log('[record] page loaded');

  // 2. 新建会话
  await page.getByRole('button', { name: '新会话' }).first().click();
  await sleep(1500);
  console.log('[record] new session created');

  // 3. 输入需求（逐字输入，录屏更自然）
  const composer = page.locator('textarea:visible').last();
  await composer.click();
  await composer.pressSequentially(args.prompt, { delay: 90 });
  await sleep(600);
  await page.screenshot({ path: path.join(monitorDir, '03-typed.png') });

  // 4. 发送
  await composer.press('Enter');
  sentAt = Date.now();
  mark('sent', args.prompt);
  console.log('[record] sent, waiting for team...');

  // 5. 等团队成员面板出现（≥2 个成员卡）
  let teamReady = false;
  for (let i = 0; i < 120; i++) {
    await sleep(2000);
    const count = await page.locator('.member-session-panel').count().catch(() => 0);
    if (count >= 2) {
      teamReady = true;
      mark('team-visible', `${count} 个成员面板`);
      break;
    }
  }
  if (!teamReady) {
    console.error('[record] WARN: 团队成员面板未出现，仍继续录制（素材可能只含编排阶段）');
  }

  // 6. 让并行执行画面稳定丰富一段时间
  await sleep(20_000);
  await page.screenshot({ path: path.join(monitorDir, '10-parallel.png') });
  mark('parallel-settled');

  // 7. 轮询所有成员面板，找到第一个「暂停按钮可见」的面板（最长 120s）。
  //    成员会话在 node_start 才发布，status 翻转为 running 后输入栏才渲染，
  //    一次性定时检查容易落在成员尚未 running 的窗口（2026-07-18 两次空跑教训）。
  const panels = page.locator('.member-session-panel');
  let target = null;
  let targetName = '';
  for (let attempt = 0; attempt < 60 && !target; attempt++) {
    const panelCount = await panels.count();
    const diag = [];
    for (let i = 0; i < panelCount; i++) {
      const p = panels.nth(i);
      const name = await p.locator('.member-header__name').innerText().catch(() => '');
      const badge = await p.locator('.member-header__status').innerText().catch(() => '');
      const btnVisible = await p.locator('button[aria-label="暂停"]').first().isVisible().catch(() => false);
      diag.push(`${name}(${badge})${btnVisible ? '+btn' : ''}`);
      if (btnVisible) {
        // 优先名字含「分析」的成员；否则记住第一个可操作的
        if (/分析/.test(name)) { target = p; targetName = name; break; }
        if (!target) { target = p; targetName = name; }
      }
    }
    if (target) break;
    if (attempt % 10 === 9) console.log(`[record] waiting pause btn: ${diag.join(' | ')}`);
    await sleep(2000);
  }

  if (target) {
    // 8. 确保面板展开（输入栏可见）
    const inputBar = target.locator('.member-input-bar');
    if (!(await inputBar.isVisible().catch(() => false))) {
      await target.locator('.member-header').click().catch(() => {});
      await sleep(1200);
    }
    // 滚到可视区中央，录屏更好看
    await target.scrollIntoViewIfNeeded().catch(() => {});
    await sleep(800);

    // 9. 点击暂停按钮（输入为空 + running 时的 stop 图标按钮，aria-label=暂停）
    const pauseBtn = target.locator('button[aria-label="暂停"]').first();
    if (await pauseBtn.isVisible().catch(() => false)) {
      await pauseBtn.click();
      mark('pause-clicked', targetName);
      // 等暂停徽章出现
      for (let i = 0; i < 15; i++) {
        await sleep(1000);
        const badge = await target.locator('.member-header__status').innerText().catch(() => '');
        if (/暂停|Paused/i.test(badge)) { mark('paused-confirmed', badge); break; }
      }
      await sleep(3000); // 让"已暂停"状态在画面里停留
      await page.screenshot({ path: path.join(monitorDir, '20-paused.png') });

      // 10. 输入注入指令并发送（paused 状态输入栏保留，注入即恢复执行）
      const input = target.locator('.member-input-bar input');
      await input.click();
      await input.pressSequentially(args.inject, { delay: 100 });
      await sleep(600);
      await page.screenshot({ path: path.join(monitorDir, '21-inject-typed.png') });
      await target.locator('button[aria-label="注入"]').first().click();
      mark('inject-sent', args.inject);
      await sleep(4000);
      await page.screenshot({ path: path.join(monitorDir, '22-injected.png') });

      // 11. 等恢复执行（徽章不再是暂停）
      for (let i = 0; i < 30; i++) {
        await sleep(1000);
        const badge = await target.locator('.member-header__status').innerText().catch(() => '');
        if (badge && !/暂停|Paused/i.test(badge)) { mark('resumed', badge); break; }
      }
    } else {
      console.error('[record] WARN: 暂停按钮不可见（成员可能已完成），跳过暂停/注入');
      mark('pause-skipped', '按钮不可见');
    }
  } else {
    console.error('[record] WARN: 无成员面板可操作，跳过暂停/注入');
  }

  // 12. 宽松完成检测（同 EP1）
  let lastText = '';
  let lastChange = Date.now();
  let lastMonitor = 0;
  let sawTeam = teamReady;
  let shotIdx = 30;

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
  mark('finished');

  // 13. 收尾：多录 5s 最终状态
  await sleep(5000);
  await page.screenshot({ path: path.join(monitorDir, '99-final.png') });

  await context.close();
  await browser.close();

  // 重命名最新 webm
  const webms = fs.readdirSync(args.outDir).filter((f) => f.endsWith('.webm'));
  if (webms.length) {
    const newest = webms
      .map((f) => ({ f, t: fs.statSync(path.join(args.outDir, f)).mtimeMs }))
      .sort((a, b) => b.t - a.t)[0].f;
    const targetPath = path.join(args.outDir, 'ep2-demo-raw.webm');
    fs.renameSync(path.join(args.outDir, newest), targetPath);
    console.log(`[record] video: ${targetPath}`);
  } else {
    console.error('[record] WARN: no webm produced');
  }

  // 动作时间戳（供剪辑对齐动画 cap 点）
  fs.writeFileSync(
    path.join(args.outDir, 'actions.json'),
    JSON.stringify({ prompt: args.prompt, inject: args.inject, actions }, null, 2),
  );
  console.log(`[record] actions: ${path.join(args.outDir, 'actions.json')}`);
  console.log(`[record] total: ${Math.round((Date.now() - sentAt) / 1000)}s`);
}

main().catch((err) => {
  console.error(`record-demo-ep2 失败：${err.message}`);
  console.error(err.stack);
  process.exit(1);
});
