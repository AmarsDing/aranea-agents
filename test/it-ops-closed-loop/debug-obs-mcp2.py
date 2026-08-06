import json
import subprocess
import threading
import time

EXE = r"C:\Users\Administrator\.local\bin\mcp-server-aliyun-observability.exe"
cmd = [EXE, "--transport", "stdio", "--access-key-id", "dummy", "--access-key-secret", "dummy"]
proc = subprocess.Popen(cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                        stderr=subprocess.PIPE, creationflags=0x08000000)

stderr_lines = []
def drain():
    for l in proc.stderr:
        stderr_lines.append(l.decode("utf-8", errors="replace").rstrip())
threading.Thread(target=drain, daemon=True).start()

msg = {"jsonrpc": "2.0", "id": 1, "method": "initialize",
       "params": {"protocolVersion": "2024-11-05", "capabilities": {},
                  "clientInfo": {"name": "probe", "version": "1.0"}}}
proc.stdin.write(json.dumps(msg).encode() + b"\n")
proc.stdin.flush()

line = proc.stdout.readline()
print("LINE1:", line[:500].decode("utf-8", errors="replace"))
if line:
    proc.stdin.write(json.dumps({"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}}).encode() + b"\n")
    proc.stdin.write(json.dumps({"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}}).encode() + b"\n")
    proc.stdin.flush()
    line2 = proc.stdout.readline()
    print("LINE2:", line2[:300].decode("utf-8", errors="replace"))
    if line2:
        resp = json.loads(line2.decode())
        tools = resp.get("result", {}).get("tools", [])
        print("TOOLS:", len(tools))
        for t in tools[:15]:
            print("  -", t.get("name"))
time.sleep(1)
proc.kill()
print("STDERR:", "\n".join(stderr_lines[-15:]))
