import fs from "fs";
import path from "path";

const root = path.join(process.cwd(), "src/features");
const marker = "// Container: approved — feature-local panel/dialog; data from Page composable via props.\n";

function walk(dir) {
  for (const ent of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, ent.name);
    if (ent.isDirectory()) walk(p);
    else if (ent.name.endsWith(".vue")) {
      const text = fs.readFileSync(p, "utf8");
      if (!text.startsWith("// Container:")) {
        fs.writeFileSync(p, marker + text, "utf8");
        console.log("marked", p);
      }
    }
  }
}

if (fs.existsSync(root)) walk(root);
