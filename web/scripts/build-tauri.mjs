#!/usr/bin/env node
/**
 * Tauri 应用打包脚本（替代原 build-electron.mjs）。
 *
 * 流程：
 *   1. 校验 web/dist/spa/index.html 存在（需先执行 pnpm build）
 *   2. cargo build --release（rust-embed 在编译期把 dist/spa 嵌入 exe）
 *   3. 拷贝 AraneaAgents.exe → web/build/tauri/AraneaAgents-win32-x64/
 *
 * 输出布局与原 Electron 产物一致（目录 + AraneaAgents.exe），
 * 因此 scripts/build-package.ps1 的 staging/frontend 结构无需变动。
 *
 * 用法：
 *   node scripts/build-tauri.mjs
 */

import { copyFile, mkdir, rm } from 'fs/promises';
import { existsSync } from 'fs';
import { spawnSync } from 'child_process';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const WEB_DIR = path.resolve(__dirname, '..');
const SRC_TAURI = path.join(WEB_DIR, 'src-tauri');

async function main() {
  const webDist = path.join(WEB_DIR, 'dist', 'spa');
  const outDir = path.join(WEB_DIR, 'build', 'tauri');
  const appDir = path.join(outDir, 'AraneaAgents-win32-x64');

  console.log('[1/3] Checking prerequisites...');
  if (!existsSync(path.join(webDist, 'index.html'))) {
    console.error('ERROR: web/dist/spa/index.html not found. Run "pnpm build" first.');
    process.exit(1);
  }
  if (!existsSync(path.join(SRC_TAURI, 'Cargo.toml'))) {
    console.error('ERROR: web/src-tauri/Cargo.toml not found.');
    process.exit(1);
  }

  console.log('[2/3] cargo build --release (this embeds dist/spa into the exe)...');
  const cargo = process.platform === 'win32' ? 'cargo.exe' : 'cargo';
  const r = spawnSync(cargo, ['build', '--release', '--locked'], {
    cwd: SRC_TAURI,
    stdio: 'inherit',
    shell: process.platform === 'win32',
  });
  if (r.status !== 0) {
    console.error(`ERROR: cargo build failed (exit ${r.status})`);
    process.exit(r.status ?? 1);
  }

  console.log('[3/3] Copying AraneaAgents.exe + icon to output directory...');
  await rm(appDir, { recursive: true, force: true });
  await mkdir(appDir, { recursive: true });
  const exe = path.join(SRC_TAURI, 'target', 'release', 'AraneaAgents.exe');
  if (!existsSync(exe)) {
    console.error('ERROR: AraneaAgents.exe not found at ' + exe);
    process.exit(1);
  }
  await copyFile(exe, path.join(appDir, 'AraneaAgents.exe'));
  // 供 NSIS 快捷方式 / DisplayIcon 使用（原 Electron 产物的 resources/app/icons/icon.ico）
  await copyFile(
    path.join(SRC_TAURI, 'icons', 'icon.ico'),
    path.join(appDir, 'icon.ico')
  );

  const { size } = await import('fs/promises').then((fs) => fs.stat(exe));
  console.log('');
  console.log('========================================');
  console.log('  Tauri app packaged successfully!');
  console.log('========================================');
  console.log(`  Output: ${path.relative(WEB_DIR, appDir)}`);
  console.log(`  Exe:    AraneaAgents.exe (${(size / 1024 / 1024).toFixed(1)} MB)`);
  console.log('');
}

main().catch((err) => {
  console.error('FATAL:', err);
  process.exit(1);
});
