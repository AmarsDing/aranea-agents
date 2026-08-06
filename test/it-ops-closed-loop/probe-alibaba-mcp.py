"""Real MCP stdio handshake: initialize + tools/list for the 3 Alibaba MCP servers.

Saves tool lists to mcp-tools-<key>.json as D3-b tool-discovery evidence.
"""
import json
import subprocess
import sys
import threading

BIN = r"C:\Users\Administrator\.local\bin"
OUT = r"f:\aranea-agents\test\it-ops-closed-loop"

SERVERS = {
    "alibaba-cloud-ops": [f"{BIN}\\alibaba-cloud-ops-mcp-server.exe"],
    "aliyun-observability-sls": [f"{BIN}\\mcp-server-aliyun-observability.exe"],
    "alibabacloud-rds-openapi": [f"{BIN}\\alibabacloud-rds-openapi-mcp-server.exe"],
}


def read_message(proc, timeout_evt):
    """Read one NDJSON MCP message (one JSON-RPC object per line)."""
    line = proc.stdout.readline()
    if not line:
        return None
    line = line.strip()
    if not line:
        return None
    return json.loads(line.decode("utf-8", errors="replace"))


def send_message(proc, msg):
    proc.stdin.write(json.dumps(msg).encode("utf-8") + b"\n")
    proc.stdin.flush()


def drain_stderr(proc):
    for _ in proc.stderr:
        pass


def probe(key, cmd):
    print(f"== {key} ==", flush=True)
    proc = subprocess.Popen(cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, creationflags=0x08000000)  # CREATE_NO_WINDOW
    t = threading.Thread(target=drain_stderr, args=(proc,), daemon=True)
    t.start()
    watchdog = threading.Timer(180.0, proc.kill)
    watchdog.start()
    try:
        send_message(proc, {
            "jsonrpc": "2.0", "id": 1, "method": "initialize",
            "params": {"protocolVersion": "2024-11-05",
                       "capabilities": {}, "clientInfo": {"name": "aranea-probe", "version": "1.0"}}
        })
        init = read_message(proc, None)
        if not init or "result" not in init:
            print("   initialize FAILED:", init, flush=True)
            return None
        server_info = init["result"].get("serverInfo", {})
        print("   server:", server_info.get("name"), server_info.get("version"), flush=True)
        send_message(proc, {"jsonrpc": "2.0", "method": "notifications/initialized", "params": {}})
        send_message(proc, {"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}})
        tools_resp = read_message(proc, None)
        tools = (tools_resp or {}).get("result", {}).get("tools", [])
        print(f"   tools discovered: {len(tools)}", flush=True)
        evidence = {
            "server_key": key,
            "command": cmd,
            "server_info": server_info,
            "protocol_version": init["result"].get("protocolVersion"),
            "tool_count": len(tools),
            "tools": [{"name": x.get("name"), "description": (x.get("description") or "")[:200]} for x in tools],
        }
        with open(f"{OUT}\\mcp-tools-{key}.json", "w", encoding="utf-8") as f:
            json.dump(evidence, f, ensure_ascii=False, indent=2)
        print(f"   [saved] mcp-tools-{key}.json", flush=True)
        for x in tools[:12]:
            print("     -", x.get("name"), flush=True)
        if len(tools) > 12:
            print(f"     ... and {len(tools) - 12} more", flush=True)
        return evidence
    finally:
        watchdog.cancel()
        proc.kill()


def main():
    results = {}
    for key, cmd in SERVERS.items():
        try:
            ev = probe(key, cmd)
            results[key] = "OK" if ev else "FAIL"
        except Exception as e:  # noqa: BLE001
            print(f"   ERROR: {e}", flush=True)
            results[key] = f"ERROR: {e}"
    print("\n== SUMMARY ==", flush=True)
    for k, v in results.items():
        print(f"  {k}: {v}", flush=True)
    if any(v != "OK" for v in results.values()):
        sys.exit(1)


if __name__ == "__main__":
    main()
