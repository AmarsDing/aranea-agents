// 生产预览冒烟门禁（P0-1 防回归，见 docs/development/91-44-deploy-readiness-and-runbook.md §3.3）
//
// 背景：构建成功 ≠ 可运行。跨 chunk 循环初始化只会在生产 bundle 运行时暴露
// （"Cannot read properties of undefined (reading 'category')" 整页白屏），
// vite build / vitest / eslint 全都发现不了。本脚本在每次部署 44 前必须通过。
//
// 用法：
//   node scripts/prod-preview-smoke.mjs            # 构建后运行（读取 dist/spa）
//   node scripts/prod-preview-smoke.mjs --url http://192.168.4.44:9301/   # 直接打已部署实例
//
// 断言（聚焦"白屏"这一类故障）：
//   1. 无 pageerror（未捕获的 JS 异常）——这是白屏的直接信号；
//   2. body 可见文本非空——SPA 真的挂载并渲染了内容。
// 不断言具体路由：本脚本跑在无后端的本地静态托管上，未登录跳转受 runtime-config
// 影响（无配置时前端允许进入 overview 空态），不属于白屏类故障。

import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { extname, join, normalize } from 'node:path';
import { chromium } from 'playwright';

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.ico': 'image/x-icon',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.svg': 'image/svg+xml',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
  '.ttf': 'font/ttf',
  '.data': 'application/octet-stream',
  '.map': 'application/json',
};

const args = process.argv.slice(2);
const urlIdx = args.indexOf('--url');
const targetUrl = urlIdx >= 0 ? args[urlIdx + 1] : null;
const spaIdx = args.indexOf('--spa-dir');
const spaDirArg = spaIdx >= 0 ? args[spaIdx + 1] : null;

function serveSpa(root) {
  const server = createServer(async (req, res) => {
    try {
      let p = decodeURIComponent(new URL(req.url, 'http://x').pathname);
      if (p === '/' || p === '') p = '/index.html';
      const file = normalize(join(root, p));
      if (!file.startsWith(normalize(root)) || !existsSync(file)) {
        // hash 路由下静态服务器只需兜底 index.html
        const html = await readFile(join(root, 'index.html'));
        res.writeHead(200, { 'content-type': MIME['.html'] });
        res.end(html);
        return;
      }
      const body = await readFile(file);
      res.writeHead(200, { 'content-type': MIME[extname(file)] || 'application/octet-stream' });
      res.end(body);
    } catch (err) {
      res.writeHead(500);
      res.end(String(err));
    }
  });
  return new Promise((resolve) => {
    server.listen(0, '127.0.0.1', () => resolve({ server, port: server.address().port }));
  });
}

let server = null;
let base = targetUrl;
if (!base) {
  const spa = spaDirArg || new URL('../dist/spa', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1');
  if (!existsSync(join(spa, 'index.html'))) {
    console.error(`[smoke] 未找到 ${spa}/index.html，先跑 npm run build`);
    process.exit(2);
  }
  ({ server, port: base } = await serveSpa(spa).then(({ server, port }) => ({
    server,
    port: `http://127.0.0.1:${port}/`,
  })));
}

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
const pageErrors = [];
const consoleErrors = [];
page.on('pageerror', (err) => pageErrors.push(err.message));
page.on('console', (msg) => {
  if (msg.type() === 'error') consoleErrors.push(msg.text());
});

let failed = false;
const fail = (msg) => {
  failed = true;
  console.error(`[smoke] FAIL: ${msg}`);
};

try {
  await page.goto(base, { waitUntil: 'networkidle', timeout: 60000 });
  await page.waitForTimeout(3000);

  const bodyText = (await page.locator('body').innerText()).trim();

  if (pageErrors.length > 0) {
    fail(`未捕获页面异常 ${pageErrors.length} 条：\n  ${pageErrors.slice(0, 5).join('\n  ')}`);
  }
  if (bodyText.length === 0) {
    fail('body 可见文本为空（白屏）');
  }
  // console.error 仅记录不判失败：无后端时 WS 重连失败等噪音属于环境性错误，
  // 与"页面是否渲染"无关；真正的白屏一定有 pageerror + 空 bodyText。
  if (consoleErrors.length > 0) {
    console.log(`[smoke] note: ${consoleErrors.length} 条 console.error（环境性，不判失败）：`);
    for (const e of consoleErrors.slice(0, 3)) console.log(`  ${e.slice(0, 160)}`);
  }

  if (!failed) {
    console.log(`[smoke] PASS ${base}  bodyText=${bodyText.length}B  pageerror=0`);
  }
} finally {
  await browser.close();
  if (server) server.close();
}

process.exit(failed ? 1 : 0);
