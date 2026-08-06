import json
import subprocess
import time

UVX = r"C:\Users\Administrator\AppData\Local\Programs\Python\Python314\Scripts\uvx.exe"
cmd = [UVX, "--python", "3.12", "--with", "python-dotenv", "mcp-server-aliyun-observability@latest"]
proc = subprocess.Popen(cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                        stderr=subprocess.PIPE, creationflags=0x08000000)
msg = {"jsonrpc": "2.0", "id": 1, "method": "initialize",
       "params": {"protocolVersion": "2024-11-05", "capabilities": {},
                  "clientInfo": {"name": "probe", "version": "1.0"}}}
proc.stdin.write(json.dumps(msg).encode() + b"\n")
proc.stdin.flush()
time.sleep(90)
proc.kill()
out, err = proc.communicate()
print("RC:", proc.returncode)
print("STDOUT:", out[:2000].decode("utf-8", errors="replace"))
s = err.decode("utf-8", errors="replace")
import re
lines = [l for l in s.splitlines() if "add_decision" not in l and "Download" not in l]
print("STDERR_FILTERED:", "\n".join(lines[-40:]))
