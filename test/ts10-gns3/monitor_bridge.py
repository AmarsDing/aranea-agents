"""TS-10 monitor bridge: NMS stand-in for the semi-physical closed loop.

Polls the GNS3-simulated enterprise network with real probes (ICMP ping,
HTTP, optional SNMP), receives real syslog from devices, and on anomaly
fires an alert into the Aranea platform chat API (like Zabbix/Alertmanager
webhook would). All raw measurements are written to evidence JSONL.
"""
import json
import socket
import subprocess
import threading
import time
import urllib.request
import urllib.error

PLATFORM = "http://localhost:8000"
TOKEN_PATH = r"F:\aranea-agents\test\it-ops-closed-loop\.token"
EVIDENCE = r"F:\aranea-agents\test\ts10-gns3\evidence"

TARGETS = {
    "edge-router-lan": "192.168.10.1",
    "pc1": "192.168.10.10",
    "pc2": "192.168.10.11",
}
HTTP_CHECK = ("edge-router-http", "http://192.168.10.1/")

syslog_lines = []
syslog_lock = threading.Lock()


def ping(ip, timeout_ms=1500):
    p = subprocess.run(["ping", "-n", "1", "-w", str(timeout_ms), ip],
                       capture_output=True, text=True, timeout=timeout_ms / 1000 + 3)
    ok = p.returncode == 0 and ("TTL=" in p.stdout or "ttl=" in p.stdout.lower())
    rtt = None
    for part in p.stdout.replace("<", " ").split():
        if part.endswith("ms") and part[:-2].replace(".", "").isdigit():
            rtt = float(part[:-2])
    return ok, rtt


def http_ok(url, timeout=3):
    try:
        with urllib.request.urlopen(url, timeout=timeout) as r:
            return r.status in (200, 301, 302, 401, 403)
    except urllib.error.HTTPError as e:
        return e.code in (401, 403)  # reachable but auth-protected
    except Exception:
        return False


def syslog_listener(port=5150):
    srv = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    srv.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    srv.bind(("0.0.0.0", port))
    while True:
        data, addr = srv.recvfrom(4096)
        line = data.decode("utf-8", "replace").strip()
        with syslog_lock:
            syslog_lines.append({"ts": time.time(), "from": addr[0], "msg": line})
            if len(syslog_lines) > 500:
                del syslog_lines[:100]


def probe_all():
    """One probe round; returns dict of results."""
    res = {"ts": time.time(), "iso": time.strftime("%Y-%m-%dT%H:%M:%S")}
    for name, ip in TARGETS.items():
        ok, rtt = ping(ip)
        res[name] = {"ok": ok, "rtt_ms": rtt}
    name, url = HTTP_CHECK
    res[name] = {"ok": http_ok(url)}
    return res


def append_jsonl(path, obj):
    with open(path, "a", encoding="utf-8") as f:
        f.write(json.dumps(obj, ensure_ascii=False) + "\n")


def start_syslog_thread():
    t = threading.Thread(target=syslog_listener, daemon=True)
    t.start()
    return t


def get_syslog_tail(n=20):
    with syslog_lock:
        return list(syslog_lines[-n:])


if __name__ == "__main__":
    start_syslog_thread()
    log = EVIDENCE + r"\monitor-probes.jsonl"
    print("monitor bridge started, logging to", log)
    while True:
        r = probe_all()
        append_jsonl(log, r)
        stat = " ".join(f"{k}={'UP' if v['ok'] else 'DOWN'}" for k, v in r.items() if isinstance(v, dict))
        print(r["iso"], stat, flush=True)
        time.sleep(5)
