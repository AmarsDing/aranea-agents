import fs from "fs";
import path from "path";

const cwd = process.cwd();
const report = JSON.parse(fs.readFileSync(path.join(cwd, "lint-report.json"), "utf8"));

function toKebab(str) {
  return str.replace(/([a-z0-9])([A-Z])/g, "$1-$2").toLowerCase();
}

function convertToModel(file, propNames) {
  let content = fs.readFileSync(file, "utf8");
  const scriptMatch = content.match(/(<script setup lang="ts">\r?\n?)([\s\S]*?)(\r?\n<\/script>)/);
  if (!scriptMatch) {
    console.log("no script", file);
    return;
  }
  let script = scriptMatch[2];
  const originalScript = script;
  const models = [];
  for (const propName of propNames) {
    const propRe = new RegExp(`^([ \\t]*)${propName}\\??\\s*:\\s*([^;\\n]+);`, "m");
    const m = script.match(propRe);
    if (!m) {
      console.log("prop not found", file, propName);
      continue;
    }
    const typeStr = m[2].trim();
    models.push({ propName, typeStr });
    script = script.replace(m[0] + "\n", "");
  }
  if (models.length === 0) return;
  for (const { propName } of models) {
    script = script.replace(new RegExp(`^[ \\t]*"update:${propName}"\\s*:\\s*\\[[^\\]]*\\];?\\r?\\n`, "m"), "");
  }
  const insertRe = /\b(withDefaults\s*\(\s*)?defineProps\s*\(/;
  const insertMatch = script.match(insertRe);
  const insertIdx = insertMatch ? insertMatch.index : 0;
  const modelLines = models.map(m => `const ${m.propName} = defineModel<${m.typeStr}>("${m.propName}", { required: true });`).join("\n") + "\n";
  script = script.slice(0, insertIdx) + modelLines + script.slice(insertIdx);
  for (const { propName } of models) {
    script = script.replace(new RegExp(`\\bprops\\.${propName}\\b`, "g"), `${propName}.value`);
  }
  content = content.replace(originalScript, script);
  fs.writeFileSync(file, content);
  console.log("converted", file, models.map(m => m.propName).join(", "));
}
function updateParentFile(file, componentBase, mutatedProps) {
  let content = fs.readFileSync(file, "utf8");
  const kebab = toKebab(componentBase);
  const tagRe = new RegExp(`<(?:${componentBase}|${kebab})\\b[^>]*>`, "sg");
  let changed = false;
  content = content.replace(tagRe, (match) => {
    let m = match;
    for (const prop of mutatedProps) {
      const pk = toKebab(prop);
      m = m.replace(new RegExp(`:${pk}="`, "g"), `v-model:${pk}="`);
    }
    if (m !== match) changed = true;
    return m;
  });
  if (changed) {
    fs.writeFileSync(file, content);
    console.log("updated parent", file, componentBase);
  }
}

const byFile = new Map();
for (const item of report) {
  for (const msg of item.messages) {
    if (msg.severity === 1 && msg.ruleId === "vue/no-mutating-props") {
      const m = msg.message.match(/Unexpected mutation of "([^"]+)" prop\./);
      if (!m) continue;
      const prop = m[1];
      const rel = item.filePath.replace(cwd, "");
      if (!byFile.has(rel)) byFile.set(rel, new Set());
      byFile.get(rel).add(prop);
    }
  }
}

for (const [rel, props] of byFile) {
  const file = path.join(cwd, rel.replace(/^\\+/, ""));
  if (!fs.existsSync(file)) {
    console.log("missing", file);
    continue;
  }
  convertToModel(file, [...props]);
}
const vueFiles = [];
function walk(dir) {
  for (const ent of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, ent.name);
    if (ent.isDirectory()) walk(p);
    else if (ent.name.endsWith(".vue")) vueFiles.push(p);
  }
}
walk(path.join(cwd, "src"));

for (const [rel, props] of byFile) {
  const childFile = path.join(cwd, rel.replace(/^\\+/, ""));
  const base = path.basename(childFile, ".vue");
  for (const parent of vueFiles) {
    if (parent === childFile) continue;
    updateParentFile(parent, base, [...props]);
  }
}
