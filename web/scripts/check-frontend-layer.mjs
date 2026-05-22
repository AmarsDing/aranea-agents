import fs from "fs";
import path from "path";

const src = path.join(process.cwd(), "src");
const violations = [];

function walk(dir) {
  for (const ent of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, ent.name);
    if (ent.isDirectory()) walk(p);
    else if (p.endsWith(".vue")) {
      const rel = path.relative(src, p).replace(/\\/g, "/");
      if (!rel.startsWith("components/")) continue;
      const text = fs.readFileSync(p, "utf8");
      if (/use\w+Store\s*\(/.test(text)) {
        violations.push(`${rel}: imports Pinia store`);
      }
      if (/from\s+['"][^'"]*features\/[^'"]+\/api['"]/.test(text)) {
        violations.push(`${rel}: imports features/*/api`);
      }
      if (/from\s+['"][^'"]*\/api['"]/.test(text) && !text.includes("Container: approved")) {
        violations.push(`${rel}: imports api (non-feature path)`);
      }
    }
  }
}

walk(path.join(src, "components"));

if (violations.length) {
  console.error("Frontend layer violations:\n" + violations.join("\n"));
  process.exit(1);
}
console.log("OK: no store/api imports in components/");
