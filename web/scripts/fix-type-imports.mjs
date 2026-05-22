import fs from "fs";
import path from "path";

const root = path.join(process.cwd(), "src");
const dirs = ["components", path.join("features", "memory"), path.join("features", "mcp")];

const typeReplacements = [
  ["features/agents/api", "features/agents/types"],
  ["features/teams/api", "features/teams/types"],
  ["features/session/api", "features/session/types"],
  ["features/platform/api", "features/platform/types"],
  ["features/chat/api", "features/chat/types"],
  ["features/monitor/api", "features/monitor/types"],
  ["features/orchestration/api", "features/orchestration/types"],
  ["features/usage/api", "features/usage/types"],
  ["features/memory/api", "features/memory/types"],
  ["features/channels/api", "features/channels/types"]
];

function fixFile(file) {
  if (file.endsWith("api.ts")) return;
  let text = fs.readFileSync(file, "utf8");
  const lines = text.split("\n");
  let changed = false;
  const out = lines.map((line) => {
    if (!line.includes("import type") && !(line.includes("import {") && line.includes(" type "))) {
      return line;
    }
    let next = line;
    for (const [from, to] of typeReplacements) {
      if (next.includes(from)) next = next.replaceAll(from, to);
    }
    if (next !== line) changed = true;
    return next;
  });
  if (changed) fs.writeFileSync(file, out.join("\n"), "utf8");
}

function walk(dir) {
  for (const ent of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, ent.name);
    if (ent.isDirectory()) walk(p);
    else if (/\.(vue|ts)$/.test(ent.name)) fixFile(p);
  }
}

for (const d of dirs) {
  const full = path.join(root, d);
  if (fs.existsSync(full)) walk(full);
}

const providerRow = path.join(root, "components/platform/ProviderModelRow.vue");
if (fs.existsSync(providerRow)) {
  let t = fs.readFileSync(providerRow, "utf8");
  t = t.replace(
    /import \{ revealProviderModelCredentials, type PlatformResource \} from "\.\.\/\.\.\/features\/platform\/types";/,
    'import { revealProviderModelCredentials } from "../../features/platform/api";\nimport type { PlatformResource } from "../../features/platform/types";'
  );
  t = t.replace(
    /import \{ revealProviderModelCredentials, type PlatformResource \} from "\.\.\/\.\.\/features\/platform\/api";/,
    'import { revealProviderModelCredentials } from "../../features/platform/api";\nimport type { PlatformResource } from "../../features/platform/types";'
  );
  fs.writeFileSync(providerRow, t, "utf8");
}

console.log("done");
