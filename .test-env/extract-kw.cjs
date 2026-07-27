// Extract keyword contexts from a snapshot dump file.
const fs = require('fs');
const file = process.argv[2];
const kws = process.argv.slice(3);
const t = fs.readFileSync(file, 'utf8');
for (const k of kws) {
  let i = -1, c = 0;
  while ((i = t.indexOf(k, i + 1)) >= 0 && c < 8) {
    console.log(`--- ${k} @${i} ---`);
    console.log(t.slice(Math.max(0, i - 100), i + 160).replace(/\n/g, ' '));
    c++;
  }
  if (c === 0) console.log(`--- ${k}: none ---`);
}
