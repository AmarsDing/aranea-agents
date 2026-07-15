#!/usr/bin/env node
/**
 * Electron 应用打包脚本（模板模式）。
 *
 * 流程：
 *   1. 用 esbuild 编译 src-electron/electron-main.ts → electron-main.js (ESM bundle)
 *   2. 从 AraneaAgents-deploy/frontend/ 复制 Electron 运行时模板
 *   3. 替换 resources/app/ 为新构建的前端 + electron-main.js
 *   4. 重命名 electron.exe → AraneaAgents.exe
 *
 * 优势：无需下载 Electron 二进制（@electron/packager 需联网下载 ~100MB），
 * 直接复用已有 deploy 包中的 Electron 运行时。
 *
 * 用法：
 *   node scripts/build-electron.mjs --platform=win32 --arch=x64
 *
 * 输出：web/dist/electron/AraneaAgents-win32-x64/
 */

import { build } from 'esbuild';
import { copyFile, mkdir, rm, writeFile, cp, readFile } from 'fs/promises';
import { existsSync } from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const WEB_DIR = path.resolve(__dirname, '..');
const REPO_ROOT = path.resolve(WEB_DIR, '..');
const TEMPLATE_DIR = path.join(REPO_ROOT, 'AraneaAgents-deploy', 'frontend');

function parseArgs() {
  const args = process.argv.slice(2);
  const opts = { platform: 'win32', arch: 'x64' };
  for (const arg of args) {
    const [k, v] = arg.replace(/^--/, '').split('=');
    if (k === 'platform') opts.platform = v;
    if (k === 'arch') opts.arch = v;
  }
  return opts;
}

async function main() {
  const { platform } = parseArgs();
  // Quasar SPA 模式构建输出到 web/dist/spa/（由 quasar.config.js 的 build.publicPath 决定）
  const webDist = path.join(WEB_DIR, 'dist', 'spa');
  // 输出到 web/build/electron/ 而非 web/dist/electron/，
  // 避免 Node cp 检测到 dest 是 src 子目录而抛错（filter 无法绕过该检查）。
  let outDir = path.join(WEB_DIR, 'build', 'electron');
  let appDir = path.join(outDir, 'AraneaAgents-win32-x64');

  console.log('[1/5] Checking prerequisites...');
  if (!existsSync(path.join(webDist, 'index.html'))) {
    console.error('ERROR: web/dist/spa/index.html not found. Run "pnpm build" first.');
    process.exit(1);
  }
  if (!existsSync(TEMPLATE_DIR)) {
    console.error('ERROR: Template directory not found: ' + TEMPLATE_DIR);
    console.error('       AraneaAgents-deploy/frontend/ must exist.');
    console.error('       Clone the repo or extract the deploy package first.');
    process.exit(1);
  }
  if (!existsSync(path.join(TEMPLATE_DIR, 'electron.exe'))) {
    console.error('ERROR: electron.exe not found in template: ' + TEMPLATE_DIR);
    process.exit(1);
  }
  console.log('  [OK] Template found: ' + TEMPLATE_DIR);

  console.log('[2/5] Cleaning output directory...');
  try {
    await rm(outDir, { recursive: true, force: true });
  } catch (e) {
    // 文件可能被另一个进程锁定（如 IDE 索引、AV 扫描），
    // fallback 到带时间戳的新目录，避免阻塞构建。
    const ts = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
    const fallbackDir = outDir + '-' + ts;
    console.warn(`  [WARN] Failed to clean ${outDir}: ${e.message}`);
    console.warn(`  [WARN] Falling back to new directory: ${fallbackDir}`);
    outDir = fallbackDir;
    appDir = path.join(outDir, 'AraneaAgents-win32-x64');
  }
  await mkdir(outDir, { recursive: true });

  console.log('[3/5] Copying Electron runtime from template...');
  // 复制整个 frontend/ 目录（electron.exe + locales + *.pak + resources/ 等）
  await cp(TEMPLATE_DIR, appDir, { recursive: true });

  // 清空 resources/app/（保留 resources/ 结构）
  const resAppDir = path.join(appDir, 'resources', 'app');
  await rm(resAppDir, { recursive: true, force: true });
  await mkdir(resAppDir, { recursive: true });

  console.log('[4/5] Compiling electron-main.ts with esbuild...');
  await build({
    entryPoints: [path.join(WEB_DIR, 'src-electron', 'electron-main.ts')],
    bundle: true,
    platform: 'node',
    format: 'esm',
    target: 'node18',
    outfile: path.join(resAppDir, 'electron-main.js'),
    external: ['electron'],
    minify: true,
    legalComments: 'none',
  });

  // 复制 quasar build 产物（index.html + 静态资源）
  // 排除所有 electron* 子目录（包括 fallback 的带时间戳目录，避免递归复制）
  await cp(webDist, resAppDir, {
    recursive: true,
    filter: (src) => {
      const rel = path.relative(webDist, src);
      if (rel === '') return true; // src === webDist 本身
      return !rel.startsWith('electron');
    },
  });

  // 复制图标
  const iconsDir = path.join(resAppDir, 'icons');
  await mkdir(iconsDir, { recursive: true });
  await copyFile(
    path.join(WEB_DIR, 'src-electron', 'icons', 'icon.png'),
    path.join(iconsDir, 'icon.png')
  );
  await copyFile(
    path.join(WEB_DIR, 'src-electron', 'icons', 'icon.ico'),
    path.join(iconsDir, 'icon.ico')
  );

  // 创建 preload 目录
  const preloadDir = path.join(resAppDir, 'preload');
  await mkdir(preloadDir, { recursive: true });
  await writeFile(
    path.join(preloadDir, 'electron-preload.cjs'),
    '"use strict";\n'
  );

  // 创建运行时配置
  const configDir = path.join(resAppDir, 'assets', 'config');
  await mkdir(configDir, { recursive: true });
  await writeFile(
    path.join(configDir, 'runtime-config.json'),
    JSON.stringify(
      { backendUrl: 'http://127.0.0.1:8000', wsOrigin: 'http://127.0.0.1:8000' },
      null,
      2
    ) + '\n'
  );

  // 创建 package.json
  const appPkg = {
    name: 'aranea-frontend',
    version: '0.1.0',
    private: true,
    type: 'module',
    main: './electron-main.js',
  };
  await writeFile(
    path.join(resAppDir, 'package.json'),
    JSON.stringify(appPkg, null, 2) + '\n'
  );

  // 清理模板中不需要的文件（lock 文件等）
  const filesToClean = ['package-lock.json', 'pnpm-lock.yaml', '.npmrc'];
  for (const f of filesToClean) {
    const fp = path.join(resAppDir, f);
    if (existsSync(fp)) await rm(fp, { force: true });
  }

  console.log('[5/5] Renaming electron.exe → AraneaAgents.exe + setting icon...');
  const oldExe = path.join(appDir, 'electron.exe');
  const newExe = path.join(appDir, 'AraneaAgents.exe');
  await rm(newExe, { force: true });
  await cp(oldExe, newExe);
  await rm(oldExe);

  // 用 rcedit 替换 AraneaAgents.exe 的图标资源
  // （electron.exe 默认图标是 Electron logo，需要替换为 aranea 图标）
  // rcedit 来自 electron-winstaller/vendor/rcedit.exe（pnpm 依赖）
  const rceditCandidates = [
    path.join(WEB_DIR, 'node_modules', '.pnpm', 'electron-winstaller@5.4.0', 'node_modules', 'electron-winstaller', 'vendor', 'rcedit.exe'),
    path.join(WEB_DIR, 'node_modules', 'electron-winstaller', 'vendor', 'rcedit.exe'),
  ];
  // 也可以从 node_modules/.pnpm/node_modules/electron-winstaller 查找
  const pnpmRoot = path.join(WEB_DIR, 'node_modules', '.pnpm', 'node_modules', 'electron-winstaller', 'vendor', 'rcedit.exe');
  rceditCandidates.push(pnpmRoot);
  let rceditExe = null;
  for (const c of rceditCandidates) {
    if (existsSync(c)) { rceditExe = c; break; }
  }
  // fallback: 全局搜索 node_modules 下的 rcedit.exe
  if (!rceditExe) {
    const { execSync } = await import('child_process');
    try {
      const found = execSync(`node -e "console.log(require.resolve('electron-winstaller/vendor/rcedit.exe'))"`, { cwd: WEB_DIR, encoding: 'utf8' }).trim();
      if (existsSync(found)) rceditExe = found;
    } catch { /* ignore */ }
  }
  if (!rceditExe) {
    console.warn('  [WARN] rcedit.exe not found; AraneaAgents.exe will keep Electron default icon');
  } else {
    const iconPath = path.join(resAppDir, 'icons', 'icon.ico');
    if (!existsSync(iconPath)) {
      console.warn('  [WARN] icon.ico not found at ' + iconPath + '; skipping rcedit');
    } else {
      const { spawnSync } = await import('child_process');
      const r = spawnSync(rceditExe, [newExe, '--set-icon', iconPath], { encoding: 'utf8' });
      if (r.status !== 0) {
        console.warn('  [WARN] rcedit failed (exit ' + r.status + '): ' + (r.stderr || r.stdout));
      } else {
        console.log('  [OK] AraneaAgents.exe icon set to ' + path.relative(WEB_DIR, iconPath));
      }
    }
  }

  console.log('');
  console.log('========================================');
  console.log('  Electron app packaged successfully!');
  console.log('========================================');
  console.log(`  Output: ${path.relative(WEB_DIR, appDir)}`);
  console.log(`  Exe:    AraneaAgents.exe`);
  console.log('');
}

main().catch((err) => {
  console.error('FATAL:', err);
  process.exit(1);
});
