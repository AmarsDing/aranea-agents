// One-off script to summarize i18n violations by directory.
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const SRC_DIR = path.resolve(__dirname, '..', 'src');

const CJK_REGEX = /[\u4e00-\u9fff]/;
const SKIP_PATHS = ['i18n/locales/', '__tests__/'];
const SKIP_FILE_SUFFIXES = ['.spec.ts', '.test.ts', '.spec.vue', '.test.vue'];

function shouldSkip(relPath) {
  if (SKIP_FILE_SUFFIXES.some((s) => relPath.endsWith(s))) return true;
  return SKIP_PATHS.some((p) => relPath.startsWith(p));
}

function walk(dir) {
  const results = [];
  for (const ent of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, ent.name);
    if (ent.isDirectory()) results.push(...walk(p));
    else if (ent.isFile() && (p.endsWith('.vue') || p.endsWith('.ts'))) results.push(p);
  }
  return results;
}

function stripBlockComments(text) {
  let out = '';
  let i = 0;
  while (i < text.length) {
    if (text[i] === '/' && text[i + 1] === '*') {
      const end = text.indexOf('*/', i + 2);
      const close = end === -1 ? text.length : end + 2;
      for (let k = i; k < close; k++) out += text[k] === '\n' ? '\n' : ' ';
      i = close;
    } else {
      out += text[i];
      i++;
    }
  }
  return out;
}

function stripLineComment(line) {
  let inSingle = false, inDouble = false, inBacktick = false;
  let i = 0;
  while (i < line.length) {
    const ch = line[i];
    if (ch === '\\' && i + 1 < line.length) { i += 2; continue; }
    if (!inDouble && !inBacktick && ch === "'") inSingle = !inSingle;
    else if (!inSingle && !inBacktick && ch === '"') inDouble = !inDouble;
    else if (!inSingle && !inDouble && ch === '`') inBacktick = !inBacktick;
    else if (!inSingle && !inDouble && !inBacktick && ch === '/' && line[i + 1] === '/') return line.slice(0, i);
    i++;
  }
  return line;
}

function stripConsoleLine(line) {
  const trimmed = line.trim();
  if (/^console\.(warn|info|error|log|debug)\s*\(/.test(trimmed)) {
    const start = line.indexOf('console.');
    if (start === -1) return line;
    let depth = 0, inSingle = false, inDouble = false, inBacktick = false, end = -1;
    for (let i = start; i < line.length; i++) {
      const ch = line[i];
      if (ch === '\\' && i + 1 < line.length) { i++; continue; }
      if (!inDouble && !inBacktick && ch === "'") inSingle = !inSingle;
      else if (!inSingle && !inBacktick && ch === '"') inDouble = !inDouble;
      else if (!inSingle && !inDouble && ch === '`') inBacktick = !inBacktick;
      else if (!inSingle && !inDouble && !inBacktick) {
        if (ch === '(') depth++;
        else if (ch === ')') { depth--; if (depth === 0) { end = i; break; } }
      }
    }
    if (end !== -1) return line.slice(0, start) + ' '.repeat(end - start + 1) + line.slice(end + 1);
    return '';
  }
  return line;
}

function splitVue(content) {
  const sections = { template: '', script: '' };
  const tplMatch = content.match(/<template[^>]*>([\s\S]*?)<\/template>/);
  if (tplMatch) sections.template = tplMatch[1];
  const scrMatch = content.match(/<script[^>]*>([\s\S]*?)<\/script>/);
  if (scrMatch) sections.script = scrMatch[1];
  return sections;
}

function stripHtmlComments(text) {
  return text.replace(/<!--[\s\S]*?-->/g, (m) => m.replace(/[^\n]/g, ' '));
}

const dirStats = new Map();
const files = walk(SRC_DIR);
let total = 0;

for (const f of files) {
  const rel = path.relative(SRC_DIR, f).replace(/\\/g, '/');
  if (shouldSkip(rel)) continue;
  const content = fs.readFileSync(f, 'utf8');
  let count = 0;
  if (f.endsWith('.vue')) {
    const sections = splitVue(content);
    const tplClean = stripHtmlComments(sections.template);
    tplClean.split('\n').forEach((line) => { if (CJK_REGEX.test(line)) count++; });
    const scrNoBlock = stripBlockComments(sections.script);
    scrNoBlock.split('\n').forEach((line) => {
      const noConsole = stripConsoleLine(stripLineComment(line));
      if (CJK_REGEX.test(noConsole)) count++;
    });
  } else {
    const noBlock = stripBlockComments(content);
    noBlock.split('\n').forEach((line) => {
      const noConsole = stripConsoleLine(stripLineComment(line));
      if (CJK_REGEX.test(noConsole)) count++;
    });
  }
  if (count > 0) {
    total += count;
    // Top-level category: components/<domain>, pages, features/<domain>, stores, etc.
    const parts = rel.split('/');
    const category = parts.length > 1 ? parts.slice(0, 2).join('/') : parts[0];
    if (!dirStats.has(category)) dirStats.set(category, { files: 0, lines: 0 });
    dirStats.get(category).files++;
    dirStats.get(category).lines += count;
  }
}

const sorted = [...dirStats.entries()].sort((a, b) => b[1].lines - a[1].lines);
console.log('Category | Files | ViolationLines');
console.log('-------|-------|-------');
for (const [cat, s] of sorted) {
  console.log(`${cat} | ${s.files} | ${s.lines}`);
}
console.log(`\nTOTAL: ${total} violation lines across ${files.length} files scanned`);
