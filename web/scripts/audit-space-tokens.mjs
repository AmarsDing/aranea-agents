#!/usr/bin/env node
// 间距令牌检查：统计 padding/margin/gap 系列声明里 px 字面量 vs var() 的比例，
// 并对已迁移域（MIGRATED）强制 0 字面量。计数口径与文档保持一致：
//   1 条形如 `padding: 12px 16px` 的声明记 1 处「档位外间距」，即使含多个 px 值。
// 用法：
//   node scripts/audit-space-tokens.mjs            # 只统计
//   node scripts/audit-space-tokens.mjs --check    # 迁移域出现字面量即退出码 1
//   node scripts/audit-space-tokens.mjs --output <path>
import { readdir, readFile, mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const srcRoot = path.join(webRoot, 'src');

// 已跑通替换范式的域：这些文件不允许再出现间距 px 字面量
const MIGRATED = [
  'src/components/approval/DecisionDock.vue',
  'src/components/approval/EvidenceRail.vue',
  'src/pages/ApprovalsPage.vue',
  'src/components/sessions/ContextIndicator.vue',
  'src/components/sessions/SessionRetentionDialog.vue',
  'src/components/sessions/SessionStatusBadge.vue',
  'src/components/monitor/AuditTable.vue',
  'src/components/monitor/MonitorAlertMetricCatalog.vue',
  'src/components/monitor/MonitorAlertRuleCard.vue',
  'src/components/monitor/RealtimeEvents.vue',
  'src/components/monitor/RunTrack.vue',
  'src/components/monitor/SelfCheckStatusPanel.vue',
  'src/components/monitor/TraceDetailDialog.vue',
  'src/components/monitor/TraceList.vue',
  'src/components/monitor/TraceSpanTree.vue',
  'src/components/monitor/TraceWaterfall.vue',
  'src/components/artifact/ArtifactRefCard.vue',
  'src/components/chat/SessionTreeNode.vue',
  'src/components/chat/TaskWorkspaceActivityPanel.vue',
  'src/components/chat/ChatHeaderUsagePanel.vue',
  'src/components/chat/SessionTreeSidebar.vue',
  'src/components/chat/observe/ObservationPanel.vue',
  'src/components/chat/tools/MediaToolDetail.vue',
  'src/components/spirit/SpiritEntry.vue',
  'src/pages/AppsPage.vue',
];

const SPACING_PROP = /^(padding|margin|gap|row-gap|column-gap)(-(top|right|bottom|left|inline|block))?$/;
const STYLE_EXT = /\.(vue|sass|scss|css)$/;

async function walk(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const nested = await Promise.all(entries.map((entry) => {
    const target = path.join(dir, entry.name);
    return entry.isDirectory() ? walk(target) : [target];
  }));
  return nested.flat();
}

const files = (await walk(srcRoot)).filter((file) => STYLE_EXT.test(file));
const inventory = [];
let totalLiteral = 0;
let totalToken = 0;

for (const file of files) {
  const source = await readFile(file, 'utf8');
  const blocks = file.endsWith('.vue')
    ? [...source.matchAll(/<style\b[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join('\n')
    : source;
  // 去掉注释，避免 // padding: 12px 之类说明计入
  const cleaned = blocks.replace(/\/\/[^\n]*|\/\*[\s\S]*?\*\//g, '');
  let literal = 0;
  let token = 0;
  const literalLines = [];
  cleaned.split('\n').forEach((line, index) => {
    const match = line.match(/^\s*([a-z-]+)\s*:\s*(.+?)\s*$/);
    if (!match || !SPACING_PROP.test(match[1])) return;
    const value = match[2].replace(/\s*!important\s*$/, '');
    const pxValues = value.match(/-?\d+(?:\.\d+)?px/g) ?? [];
    if (pxValues.length) {
      literal += 1;
      literalLines.push({ line: index + 1, prop: match[1], value });
    } else if (value.includes('var(--nt-space') || value.includes('var(--space')) {
      token += 1;
    }
  });
  totalLiteral += literal;
  totalToken += token;
  if (literal || token) {
    inventory.push({ file: path.relative(webRoot, file).replaceAll('\\', '/'), literal, token, literalLines });
  }
}

const ranked = inventory.filter((entry) => entry.literal).sort((a, b) => b.literal - a.literal);
const report = {
  scope: '间距声明（padding/margin/gap 系列）按声明计数：含 px 字面量记 1 处 literal，走 var() 记 1 处 token。',
  files: inventory.length,
  totalLiteral,
  totalToken,
  tokenShare: totalToken + totalLiteral ? Math.round((totalToken / (totalToken + totalLiteral)) * 1000) / 10 : 0,
  migrated: MIGRATED.map((file) => ({
    file,
    literal: inventory.find((entry) => entry.file === file)?.literal ?? 0,
  })),
  topFiles: ranked.slice(0, 20),
};

console.log(JSON.stringify({
  files: report.files,
  totalLiteral: report.totalLiteral,
  totalToken: report.totalToken,
  tokenShare: report.tokenShare,
  migrated: report.migrated,
}, null, 2));

const outputIndex = process.argv.indexOf('--output');
if (outputIndex >= 0 && process.argv[outputIndex + 1]) {
  const output = path.resolve(webRoot, process.argv[outputIndex + 1]);
  await mkdir(path.dirname(output), { recursive: true });
  await writeFile(output, `${JSON.stringify(report, null, 2)}\n`);
  console.log(`Report: ${output}`);
}

if (process.argv.includes('--check')) {
  const failures = report.migrated.filter((entry) => entry.literal > 0);
  // 令牌本体必须存在，否则 var() 全部失效且静默降级
  const tokens = await readFile(path.join(srcRoot, 'css/theme/_nt-tokens.css'), 'utf8');
  for (const step of [1, 2, 3, 4, 6, 8, 12, 16]) {
    if (!new RegExp(`--nt-space-${step}:`).test(tokens)) failures.push(`missing --nt-space-${step} token`);
  }
  for (const key of ['page', 'section', 'panel', 'cell']) {
    if (!new RegExp(`--nt-space-${key}:`).test(tokens)) failures.push(`missing semantic token --nt-space-${key}`);
  }
  const offenders = report.topFiles.filter((entry) => MIGRATED.includes(entry.file) && entry.literal);
  for (const entry of offenders) failures.push(`${entry.file}: ${entry.literal} spacing literals in migrated domain`);
  if (failures.length) {
    console.error(failures.map((item) => (typeof item === 'string' ? item : `${item.file}: ${item.literal} spacing literals`)).join('\n'));
    process.exitCode = 1;
  } else console.log('Space tokens passed: --nt-space-* complete and migrated domains free of spacing literals.');
}
