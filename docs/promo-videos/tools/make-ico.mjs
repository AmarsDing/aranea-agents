#!/usr/bin/env node
/**
 * make-ico.mjs · 把 aranea.png 转成 Windows 桌面图标 aranea.ico（多尺寸）
 * 用法：node make-ico.mjs --in <png> --out <ico>
 */
import fs from 'node:fs';
import path from 'node:path';
import pngToIco from 'png-to-ico';

const args = {};
for (let i = 2; i < process.argv.length; i++) {
  if (process.argv[i] === '--in') args.in = process.argv[++i];
  else if (process.argv[i] === '--out') args.out = process.argv[++i];
}
if (!args.in || !args.out) {
  console.error('用法：node make-ico.mjs --in <png> --out <ico>');
  process.exit(1);
}

const buf = await pngToIco(args.in);
fs.mkdirSync(path.dirname(path.resolve(args.out)), { recursive: true });
fs.writeFileSync(args.out, buf);
console.log(JSON.stringify({ out: path.resolve(args.out), bytes: buf.length }));
