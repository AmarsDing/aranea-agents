const {execSync} = require('child_process');
const fs = require('fs');
try {
  execSync('npx stylelint "src/**/*.{vue,css}" --config .stylelintrc.json -f json > f:/aranea-agents/stylelint-result.json 2>&1', {
    encoding: 'utf8',
    maxBuffer: 50 * 1024 * 1024,
    shell: true
  });
} catch(e) {
  // stylelint exits with code 1 when there are warnings
}
const r = fs.readFileSync('f:/aranea-agents/stylelint-result.json', 'utf8').trim();
if (!r || r.startsWith('[') === false) {
  console.log('No valid output');
  process.exit(1);
}
const arr = JSON.parse(r);
const rules = {};
const files = {};
const hexDetails = {};
arr.forEach(f => {
  if (f.ignored) return;
  const short = f.source.replace(/\\/g, '/').replace(/.*\/web\/src\//, '');
  files[short] = (files[short] || 0) + f.warnings.length;
  f.warnings.forEach(w => {
    rules[w.rule] = (rules[w.rule] || 0) + 1;
    if (w.rule === 'color-no-hex') {
      const match = w.text.match(/#[0-9a-fA-F]{3,8}/);
      if (match) {
        const hex = match[0].toLowerCase();
        hexDetails[hex] = (hexDetails[hex] || 0) + 1;
      }
    }
  });
});
const out = [];
out.push('Total files: ' + arr.filter(f => !f.ignored).length);
out.push('Total warnings: ' + arr.filter(f => !f.ignored).reduce((s, f) => s + f.warnings.length, 0));
out.push('\n=== By rule ===');
Object.entries(rules).sort((a, b) => b[1] - a[1]).forEach(([k, v]) => out.push(v + '\t' + k));
out.push('\n=== Top 20 files ===');
Object.entries(files).sort((a, b) => b[1] - a[1]).slice(0, 20).forEach(([k, v]) => out.push(v + '\t' + k));
out.push('\n=== Remaining hex colors ===');
Object.entries(hexDetails).sort((a, b) => b[1] - a[1]).forEach(([k, v]) => out.push(v + '\t' + k));
fs.writeFileSync('f:/aranea-agents/stylelint-summary.txt', out.join('\n'));
console.log('Done');
