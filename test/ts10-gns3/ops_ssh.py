"""TS-10 ops jump-host channel: audited SSH exec into simulated network devices.

Agents (via shell_exec) call:
    python ops_ssh.py <host> "<command>"

Credentials are injected from the ops vault (here: scenario constants
root/123456, matching the uniform lab credential policy admin/123456).
Every invocation is appended to evidence/device-ops-audit.jsonl with
timestamp, caller cwd, command, rc, stdout/stderr -> real audit trail.
"""
import json
import os
import sys
import time

import paramiko

AUDIT = r"F:\aranea-agents\test\ts10-gns3\evidence\device-ops-audit.jsonl"
CREDS = {"username": "root", "password": "123456"}


def main():
    if len(sys.argv) < 3:
        print("usage: ops_ssh.py <host> <command>")
        return 2
    host, cmd = sys.argv[1], sys.argv[2]
    rec = {"ts": time.strftime("%Y-%m-%dT%H:%M:%S"), "host": host, "cmd": cmd, "cwd": os.getcwd()}
    cli = paramiko.SSHClient()
    cli.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    try:
        cli.connect(host, username=CREDS["username"], password=CREDS["password"],
                    timeout=15, banner_timeout=15, auth_timeout=15,
                    allow_agent=False, look_for_keys=False)
        stdin, stdout, stderr = cli.exec_command(cmd, timeout=120)
        out = stdout.read().decode("utf-8", "replace")
        err = stderr.read().decode("utf-8", "replace")
        rc = stdout.channel.recv_exit_status()
        rec.update(rc=rc, stdout=out[-4000:], stderr=err[-2000:])
        print(out)
        if err:
            print(err, file=sys.stderr)
        return rc
    except Exception as e:  # noqa: BLE001
        rec.update(rc=-1, error=str(e))
        print(f"ops_ssh error: {e}", file=sys.stderr)
        return 1
    finally:
        try:
            cli.close()
        except Exception:
            pass
        os.makedirs(os.path.dirname(AUDIT), exist_ok=True)
        with open(AUDIT, "a", encoding="utf-8") as f:
            f.write(json.dumps(rec, ensure_ascii=False) + "\n")


if __name__ == "__main__":
    sys.exit(main())
