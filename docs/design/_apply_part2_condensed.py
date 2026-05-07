# -*- coding: utf-8 -*-
from pathlib import Path

root = Path(__file__).resolve().parent
main_path = root / "platform-architecture.md"
condensed = (root / "_part2_condensed.md").read_text(encoding="utf-8")
if not condensed.endswith("\n"):
    condensed += "\n"

text = main_path.read_text(encoding="utf-8")
end_m = "\n# 第三篇 平台架构与目标态实现全文"
idx_end = text.find(end_m)
if idx_end < 0:
    raise SystemExit("end marker not found")

candidates = []
for pat in (
    "\n\uFEFF# 第二篇 LLM Gateway",
    "\n# 第二篇 LLM Gateway",
    "\uFEFF# 第二篇 LLM Gateway",
):
    p = 0
    while True:
        j = text.find(pat, p, idx_end)
        if j < 0:
            break
        off = j + (1 if pat.startswith("\n") else 0)
        if pat.startswith("\n\uFEFF"):
            off = j + len("\n\uFEFF")
        elif pat.startswith("\n#"):
            off = j + 1
        candidates.append(off)
        p = j + 1

if not candidates:
    raise SystemExit("no second-parts heading")

start = min(candidates)
prefix = text[:start]
suffix = text[idx_end:]

new_text = prefix + condensed + suffix

# collapse 3+ trailing --- before second part to a single blank + ---
plines = prefix.splitlines(True)
dash_run = 0
i = len(plines) - 1
while i >= 0 and plines[i].strip() == "":
    i -= 1
while i >= 0 and plines[i].strip() == "---":
    dash_run += 1
    i -= 1
if dash_run > 1:
    prefix = "".join(plines[: i + 1]) + "\n---\n\n"
    new_text = prefix + condensed + suffix

main_path.write_text(new_text, encoding="utf-8")
(root / "_part2_condensed.md").unlink(missing_ok=True)
Path(__file__).unlink(missing_ok=True)
print("ok start", start)
