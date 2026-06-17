/**
 * i18n coverage check — scans .vue and .ts files for hardcoded Chinese
 * characters that should be migrated to i18n keys.
 *
 * Run: node scripts/check-i18n.mjs
 * Or:   pnpm check:i18n
 *
 * Modes:
 *   (default)          Check for new violations not in baseline.
 *   --update-baseline  Rewrite the baseline with current violations.
 *
 * Exclusions (legitimate Chinese, not violations):
 *   - Files under src/i18n/locales/ (locale definitions themselves)
 *   - Line comments (slash-slash) and block comments (slash-star)
 *   - HTML comments in template (angle-bang-dash-dash)
 *   - console.* statements (console.warn/info/error/log per project rules)
 *   - Test files (*.spec.ts, *.test.ts, __tests__/)
 *
 * Detection: Unicode CJK range \u4e00-\u9fff (covers common Chinese)
 *
 * Baseline: scripts/i18n-baseline.json lists files with pre-existing
 * violations (tech debt). The check fails only on:
 *   1. Violations in files NOT in the baseline (new files with Chinese)
 *   2. Violation counts in baseline files that EXCEED the recorded count
 *      (new hardcoded strings added to already-debt-laden files)
 *
 * Exit codes: 0 = pass, 1 = new violations found
 */
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const SRC_DIR = path.resolve(__dirname, '..', 'src');
const BASELINE_PATH = path.resolve(__dirname, 'i18n-baseline.json');

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

function stripHtmlComments(text) {
  return text.replace(/<!--[\s\S]*?-->/g, (m) => m.replace(/[^\n]/g, ' '));
}

function splitVue(content) {
  const sections = { template: '', script: '' };
  // Top-level <template> is the first <template ...> in the file; its closing
  // </template> is the LAST one (Vue SFCs have a single root template, but it
  // may contain nested <template> slot tags). Find by matching the first open
  // and the last close.
  const tplOpen = content.search(/<template[^>]*>/);
  if (tplOpen !== -1) {
    const openEnd = content.indexOf('>', tplOpen) + 1;
    const tplClose = content.lastIndexOf('</template>');
    if (tplClose > openEnd) {
      sections.template = content.slice(openEnd, tplClose);
    }
  }
  // <script> section: first <script ...> ... </script>
  const scrOpen = content.search(/<script[^>]*>/);
  if (scrOpen !== -1) {
    const openEnd = content.indexOf('>', scrOpen) + 1;
    const scrClose = content.indexOf('</script>', openEnd);
    if (scrClose > openEnd) {
      sections.script = content.slice(openEnd, scrClose);
    }
  }
  return sections;
}

function countViolations(filePath) {
  const content = fs.readFileSync(filePath, 'utf8');
  const violations = [];
  if (filePath.endsWith('.vue')) {
    const sections = splitVue(content);
    const tplClean = stripHtmlComments(sections.template);
    tplClean.split('\n').forEach((line, idx) => {
      if (CJK_REGEX.test(line)) {
        violations.push({ line: idx + 1, section: 'template', snippet: line.trim().slice(0, 120) });
      }
    });
    const scrNoBlock = stripBlockComments(sections.script);
    scrNoBlock.split('\n').forEach((line, idx) => {
      const noConsole = stripConsoleLine(stripLineComment(line));
      if (CJK_REGEX.test(noConsole)) {
        violations.push({ line: idx + 1, section: 'script', snippet: noConsole.trim().slice(0, 120) });
      }
    });
  } else {
    const noBlock = stripBlockComments(content);
    noBlock.split('\n').forEach((line, idx) => {
      const noConsole = stripConsoleLine(stripLineComment(line));
      if (CJK_REGEX.test(noConsole)) {
        violations.push({ line: idx + 1, section: 'ts', snippet: noConsole.trim().slice(0, 120) });
      }
    });
  }
  return violations;
}

function loadBaseline() {
  if (!fs.existsSync(BASELINE_PATH)) return {};
  try {
    return JSON.parse(fs.readFileSync(BASELINE_PATH, 'utf8'));
  } catch {
    return {};
  }
}

function saveBaseline(baseline) {
  const sorted = {};
  for (const k of Object.keys(baseline).sort()) sorted[k] = baseline[k];
  fs.writeFileSync(BASELINE_PATH, JSON.stringify(sorted, null, 2) + '\n', 'utf8');
}

function main() {
  const updateBaseline = process.argv.includes('--update-baseline');
  if (!fs.existsSync(SRC_DIR)) {
    console.error(`ERROR: src directory not found at ${SRC_DIR}`);
    process.exit(2);
  }

  const files = walk(SRC_DIR);
  const current = {}; // relPath -> count
  const currentDetails = {}; // relPath -> [{line, section, snippet}]

  for (const f of files) {
    const rel = path.relative(SRC_DIR, f).replace(/\\/g, '/');
    if (shouldSkip(rel)) continue;
    const viols = countViolations(f);
    if (viols.length > 0) {
      current[rel] = viols.length;
      currentDetails[rel] = viols;
    }
  }

  if (updateBaseline) {
    saveBaseline(current);
    const totalFiles = Object.keys(current).length;
    const totalViolations = Object.values(current).reduce((a, b) => a + b, 0);
    console.log(`Baseline written: ${totalViolations} violation(s) across ${totalFiles} file(s).`);
    console.log(`Path: ${BASELINE_PATH}`);
    console.log('Re-run without --update-baseline to check for new violations.');
    process.exit(0);
  }

  const baseline = loadBaseline();
  const newFiles = [];
  const increasedFiles = [];
  let newViolationsCount = 0;

  for (const [file, count] of Object.entries(current)) {
    const baseCount = baseline[file] || 0;
    if (baseCount === 0) {
      newFiles.push({ file, count, details: currentDetails[file] });
      newViolationsCount += count;
    } else if (count > baseCount) {
      increasedFiles.push({ file, count, baseCount, details: currentDetails[file] });
      newViolationsCount += count - baseCount;
    }
  }

  const fixedFiles = Object.keys(baseline).filter((f) => !current[f]);
  const reducedFiles = Object.entries(current)
    .filter(([f, c]) => baseline[f] && c < baseline[f])
    .map(([f, c]) => ({ file: f, count: c, baseCount: baseline[f] }));

  // Report
  if (newFiles.length === 0 && increasedFiles.length === 0) {
    const totalCurrent = Object.values(current).reduce((a, b) => a + b, 0);
    const totalBase = Object.values(baseline).reduce((a, b) => a + b, 0);
    console.log(`OK: no new hardcoded Chinese violations.`);
    console.log(`  Current tech debt: ${totalCurrent} violation(s) in ${Object.keys(current).length} file(s) (baseline: ${totalBase}).`);
    if (fixedFiles.length > 0) console.log(`  Files fully fixed since baseline: ${fixedFiles.length}`);
    if (reducedFiles.length > 0) console.log(`  Files with reduced violations: ${reducedFiles.length}`);
    process.exit(0);
  }

  console.error(`FAIL: ${newViolationsCount} NEW hardcoded Chinese violation(s) found.\n`);
  console.error('Migrate these to i18n keys (web/src/i18n/locales/zh-CN.ts and en-US.ts).\n');

  if (newFiles.length > 0) {
    console.error(`New files with hardcoded Chinese (${newFiles.length}):`);
    for (const { file, count, details } of newFiles) {
      console.error(`  ${file} (${count} violation(s)):`);
      for (const d of details.slice(0, 5)) {
        console.error(`    L${d.line} [${d.section}]: ${d.snippet}`);
      }
      if (details.length > 5) console.error(`    ... and ${details.length - 5} more`);
    }
    console.error('');
  }

  if (increasedFiles.length > 0) {
    console.error(`Existing files with MORE violations than baseline (${increasedFiles.length}):`);
    for (const { file, count, baseCount, details } of increasedFiles) {
      console.error(`  ${file}: ${count} now (was ${baseCount}, +${count - baseCount})`);
      // Show the last N violations (likely the new ones at the end)
      const newOnes = details.slice(baseCount);
      for (const d of newOnes.slice(0, 5)) {
        console.error(`    L${d.line} [${d.section}]: ${d.snippet}`);
      }
      if (newOnes.length > 5) console.error(`    ... and ${newOnes.length - 5} more`);
    }
    console.error('');
  }

  console.error(`To update the baseline (after intentional migrations): node scripts/check-i18n.mjs --update-baseline`);
  process.exit(1);
}

main();
