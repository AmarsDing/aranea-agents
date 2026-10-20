#!/usr/bin/env node
// web/scripts/check-v2-event-contract.mjs
//
// v2 事件契约守门（2026-10-20 P3-8 重建；原 check-envelope-contract.ts 已失效
// 并删除——它校验的前端 envelope 类型在 v2 重构中不复存在，且从未接入 npm
// scripts）。本脚本直接以 Go 源码为后端真相源，纯文本解析、无依赖：
//
//   1. v2 EventKind 双向对齐：
//      internal/biz/event.go + event_system.go 的 `EventKindXxx EventKind = "..."`
//      常量 ↔ web/src/features/chat/v2Types.ts 的 EventKind union。
//      两端必须一一对应（后端发了前端不认的 kind 会静默丢失事件；前端声明了
//      后端不发的 kind 是死代码）。
//   2. WS wire 载荷字段对齐：
//      internal/server/ws_v2_wire.go 每个 struct 的 json tag ↔ v2Types.ts 中
//      映射 TS interface 的属性集合。方向双向（缺失/多余都报错；TS 属性以
//      `?:` 标记可选不影响检查）。PascalCase / snake_case 均由 Go json tag
//      决定，脚本不做大小写假设。
//   3. Monitor 事件类型对齐：
//      internal/event/contract/monitor_event.go 的 MonitorEventType 常量 ↔
//      web/src/realtime/monitorEvent.ts 的 MonitorEventType union。
//
// 退出码：0 = 全绿；1 = 存在漂移（打印双向差异明细）。
// 用法：pnpm check:contract（也挂入 pnpm lint 链）。

import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), '..', '..');

function readGo(rel) {
  return readFileSync(join(repoRoot, rel), 'utf8');
}
function readWeb(rel) {
  return readFileSync(join(repoRoot, 'web', rel), 'utf8');
}

// ---------------------------------------------------------------------------
// 解析器

/** Go: `SomeName SomeType = "value"` 常量块中提取 value 集合。 */
function goConstValues(src, typeName) {
  const re = new RegExp(`\\b\\w+\\s+${typeName}\\s*=\\s*"([^"]+)"`, 'g');
  return new Set([...src.matchAll(re)].map((m) => m[1]));
}

/** Go: 解析全部 `type Xxx struct { ... }` 的 json tag 字段名（忽略 `-`）。 */
function goWireStructs(src) {
  const structs = new Map();
  const reStruct = /type (\w+) struct \{([\s\S]*?)\n\}/g;
  for (const m of src.matchAll(reStruct)) {
    const fields = [];
    for (const tag of m[2].matchAll(/json:"([^"]+)"/g)) {
      const name = tag[1].split(',')[0];
      if (name && name !== '-') fields.push(name);
    }
    structs.set(m[1], fields);
  }
  return structs;
}

/** TS: 解析 `export interface Xxx { ... }` / `export type Xxx = { ... }` 的属性名集合。 */
function tsInterfaces(src) {
  const ifaces = new Map();
  const reIface = /export (?:interface|type) (\w+)(?:\s*=\s*)?\s*\{([\s\S]*?)\n\}/g;
  for (const m of src.matchAll(reIface)) {
    // 去注释后按 `name?:` / `name:` 模式提取属性名
    const body = m[2].replace(/\/\/[^\n]*/g, '').replace(/\/\*[\s\S]*?\*\//g, '');
    const props = [...body.matchAll(/([A-Za-z_][A-Za-z0-9_]*)\??\s*:/g)].map((p) => p[1]);
    ifaces.set(m[1], props);
  }
  return ifaces;
}

/** TS: 解析 `export type X = | 'a' | 'b' ;` 字符串字面量 union。
 *  先剥离两种注释——union 内的注释可能含 `;`，会让朴素正则提前截断。 */
function tsStringUnion(src, typeName) {
  const stripped = src.replace(/\/\*[\s\S]*?\*\//g, '').replace(/\/\/[^\n]*/g, '');
  const re = new RegExp(`export type ${typeName} =([\\s\\S]*?);`, 'g');
  const m = re.exec(stripped);
  if (!m) return null;
  return new Set([...m[1].matchAll(/'([^']+)'/g)].map((s) => s[1]));
}

// ---------------------------------------------------------------------------
// 差异报告

const failures = [];

function diffSets(name, backendSet, frontendSet) {
  const onlyBackend = [...backendSet].filter((x) => !frontendSet.has(x));
  const onlyFrontend = [...frontendSet].filter((x) => !backendSet.has(x));
  if (onlyBackend.length === 0 && onlyFrontend.length === 0) {
    console.log(`  [ok] ${name}: backend=${backendSet.size} frontend=${frontendSet.size}`);
    return;
  }
  failures.push(name);
  console.error(`  [DRIFT] ${name}:`);
  for (const x of onlyBackend) console.error(`    - backend-only (前端缺失): ${x}`);
  for (const x of onlyFrontend) console.error(`    - frontend-only (后端不存在): ${x}`);
}

// ---------------------------------------------------------------------------
// 检查 1: v2 EventKind 双向对齐

console.log('[1/3] v2 EventKind ↔ v2Types.ts EventKind union');
const goEventSrc = readGo('internal/biz/event.go') + '\n' + readGo('internal/biz/event_system.go');
const goKinds = goConstValues(goEventSrc, 'EventKind');
const v2TypesSrc = readWeb('src/features/chat/v2Types.ts');
const tsKinds = tsStringUnion(v2TypesSrc, 'EventKind');
if (!tsKinds) {
  failures.push('EventKind union');
  console.error('  [DRIFT] 无法在 v2Types.ts 中解析 `export type EventKind = ...`');
} else {
  diffSets('v2 EventKind', goKinds, tsKinds);
}

// ---------------------------------------------------------------------------
// 检查 2: WS wire struct json tag ↔ TS interface 属性

console.log('[2/3] ws_v2_wire.go 载荷字段 ↔ v2Types.ts payload interfaces');
const wireSrc = readGo('internal/server/ws_v2_wire.go');
const goStructs = goWireStructs(wireSrc);
const tsIfaces = tsInterfaces(v2TypesSrc);
// SkillCatalogEntry 定义在 features/skills/types.ts（skill.catalog 复用该契约），
// 与 v2Types 的 interface 合并后参与字段对齐。
for (const [name, props] of tsInterfaces(readWeb('src/features/skills/types.ts'))) {
  tsIfaces.set(name, props);
}

// wire struct → TS interface（shape 相同的 payload 允许映射多次，如
// stepEventWire 同时用于 step.created / step.updated）。
// 第三列指定 Go struct 所在文件：默认 ws_v2_wire.go；个别领域类型本身即
// wire 定义（json tag 直接锁在线上）时无 *Wire 包装，直接解析领域文件。
const WIRE_TO_TS = [
  ['taskWire', 'Task'],
  ['turnWire', 'Turn'],
  ['stepWire', 'Step'],
  ['memberInfoWire', 'MemberInfo'],
  ['teamStageWire', 'TeamStage'],
  ['teamRunWire', 'TeamRun'],
  ['memberSessionWire', 'MemberSession'],
  ['planBoardWire', 'PlanBoard'],
  ['planStepWire', 'PlanStep'],
  // biz.DeliverableContract 自带 snake_case json tag（本身即 wire 定义），
  // ws_v2_wire.go 中无包装 struct —— 解析 biz 源文件（ws_v2_wire.go:190-192）。
  ['DeliverableContract', 'DeliverableContract', 'internal/biz/deliverable_contract.go'],
  ['stepResultWire', 'StepResult'],
  ['memberReportWire', 'MemberReport'],
  ['stepErrorWire', 'StepError'],
  ['tokenUsageWire', 'TokenUsage'],
  ['graphStageWire', 'GraphStage'],
  ['graphNodeWire', 'GraphNode'],
  ['taskEventWire', 'TaskEventPayload'],
  ['turnEventWire', 'TurnEventPayload'],
  ['stepStreamingEventWire', 'StepStreamingPayload'],
  ['stepEventWire', 'StepCreatedPayload'],
  ['stepEventWire', 'StepUpdatedPayload'],
  ['teamStageEventWire', 'TeamStageEventPayload'],
  ['teamRunEventWire', 'TeamRunEventPayload'],
  ['memberSessionEventWire', 'MemberSessionEventPayload'],
  ['planBoardEventWire', 'PlanBoardEventPayload'],
  ['planStepEventWire', 'PlanStepEventPayload'],
  ['planStepSkippedEventWire', 'PlanStepSkippedPayload'],
  ['graphStageEventWire', 'GraphStageEventPayload'],
  ['graphNodeEventWire', 'GraphNodeEventPayload'],
  ['runStatusEventWire', 'RunStatusEventPayload'],
  ['heartbeatEventWire', 'HeartbeatEventPayload'],
  ['systemNoticeEventWire', 'SystemNoticeEventPayload'],
  // skill.catalog 条目契约在 features/skills/types.ts（snake_case）。
  ['skillCatalogEntryWire', 'SkillCatalogEntry'],
  ['skillCatalogEventWire', 'SkillCatalogEventPayload'],
];

let wireChecked = 0;
for (const [goStruct, tsIface, srcFile] of WIRE_TO_TS) {
  const sourceStructs = srcFile ? goWireStructs(readGo(srcFile)) : goStructs;
  const goFields = sourceStructs.get(goStruct);
  if (!goFields) {
    failures.push(`${goStruct}`);
    console.error(`  [DRIFT] ${goStruct}: ws_v2_wire.go 中不存在（脚本映射表过期？）`);
    continue;
  }
  const tsProps = tsIfaces.get(tsIface);
  if (!tsProps) {
    failures.push(`${tsIface}`);
    console.error(`  [DRIFT] ${tsIface}: v2Types.ts 中不存在（脚本映射表过期？）`);
    continue;
  }
  diffSets(`${goStruct} ↔ ${tsIface}`, new Set(goFields), new Set(tsProps));
  wireChecked++;
}
if (wireChecked === WIRE_TO_TS.length) {
  console.log(`  [ok] ${wireChecked} 组 wire↔TS 字段对齐`);
}

// ---------------------------------------------------------------------------
// 检查 3: MonitorEventType 对齐

console.log('[3/3] MonitorEventType ↔ monitorEvent.ts union');
const monitorGo = readGo('internal/event/contract/monitor_event.go');
const goMonitor = goConstValues(monitorGo, 'MonitorEventType');
const monitorTs = tsStringUnion(readWeb('src/realtime/monitorEvent.ts'), 'MonitorEventType');
if (!monitorTs) {
  failures.push('MonitorEventType union');
  console.error('  [DRIFT] 无法在 monitorEvent.ts 中解析 `export type MonitorEventType = ...`');
} else {
  diffSets('MonitorEventType', goMonitor, monitorTs);
}

// ---------------------------------------------------------------------------

if (failures.length > 0) {
  console.error(`\nv2 事件契约检查失败：${failures.length} 处漂移。`);
  console.error('修复方向：后端发什么前端收什么（Go json tag / EventKind 常量 为真相源）。');
  process.exit(1);
}
console.log('\nv2 事件契约检查通过 ✓');
