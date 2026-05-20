import re
import pathlib

for p in pathlib.Path("pkg/trpc-agent-go/tool/mcp").glob("*_test.go"):
    t = p.read_text(encoding="utf-8")

    def repl(m: re.Match) -> str:
        inner = m.group(1)
        if inner.rstrip().endswith('"", nil'):
            return m.group(0)
        return f'newMCPSessionManager({inner}, "", nil)'

    t2 = re.sub(r"newMCPSessionManager\(([^()]*(?:\([^()]*\)[^()]*)*)\)", repl, t)
    if t2 != t:
        p.write_text(t2, encoding="utf-8")
        print("updated", p)
