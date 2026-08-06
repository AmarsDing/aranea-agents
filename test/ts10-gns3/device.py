"""TS-10 device automation: GNS3 telnet console + SSH helpers for OpenWrt nodes.

Console: raw TCP to 127.0.0.1:<gns3_console_port> (GNS3 qemu console_type=telnet).
The GNS3 telnet console performs minimal IAC negotiation; we strip IAC sequences.
SSH: paramiko to the device IP through the GNS3 cloud/loopback bridge.
"""
import re
import socket
import time

import paramiko

IAC = bytes([255])


def _strip_telnet_iac(data: bytes) -> bytes:
    # Remove IAC sequences: IAC (255) followed by cmd (251-254) + option byte,
    # or IAC + single-byte command.
    out = bytearray()
    i = 0
    while i < len(data):
        if data[i] == 255 and i + 1 < len(data):
            cmd = data[i + 1]
            if cmd in (251, 252, 253, 254) and i + 2 < len(data):
                i += 3
                continue
            i += 2
            continue
        out.append(data[i])
        i += 1
    return bytes(out)


class Console:
    """Interactive telnet console to a GNS3 node."""

    def __init__(self, port, host="127.0.0.1", timeout=300):
        self.sock = socket.create_connection((host, port), timeout=30)
        self.sock.settimeout(5)
        self.buf = b""
        self.timeout = timeout

    def read_until(self, patterns, timeout=None):
        """patterns: str or list of str/regex. Returns (matched_index, text)."""
        if isinstance(patterns, str):
            patterns = [patterns]
        timeout = timeout or self.timeout
        deadline = time.time() + timeout
        while time.time() < deadline:
            try:
                chunk = self.sock.recv(4096)
                if chunk:
                    self.buf += _strip_telnet_iac(chunk)
                else:
                    time.sleep(0.3)
            except socket.timeout:
                pass
            text = self.buf.decode("utf-8", "replace")
            for idx, pat in enumerate(patterns):
                if re.search(pat, text):
                    return idx, text
        raise TimeoutError(f"console read_until timeout, buf tail: {self.buf[-500:]!r}")

    def send(self, s=""):
        self.sock.sendall(s.encode() + b"\r\n")
        time.sleep(0.4)

    def send_raw(self, s):
        self.sock.sendall(s.encode())
        time.sleep(0.3)

    def drain(self, seconds=2):
        end = time.time() + seconds
        while time.time() < end:
            try:
                chunk = self.sock.recv(4096)
                if chunk:
                    self.buf += _strip_telnet_iac(chunk)
            except socket.timeout:
                pass
        text = self.buf.decode("utf-8", "replace")
        self.buf = b""
        return text

    def close(self):
        try:
            self.sock.close()
        except Exception:
            pass


def wait_boot(con: Console, timeout=420):
    """Wait for OpenWrt boot to finish; activate console shell.

    GNS3 telnet console is a live serial stream with no scrollback replay,
    so we must actively send CRLF to elicit a prompt from an idle shell.
    """
    patterns = ["Please press Enter to activate", r"root@[^#]*#", r"bash[^#]*#", r"/ #"]
    deadline = time.time() + timeout
    while time.time() < deadline:
        con.send_raw("\r\n")
        try:
            idx, _ = con.read_until(patterns, timeout=15)
        except TimeoutError:
            continue  # guest still booting quietly; poke again
        if idx == 0:
            con.send_raw("\r\n")
            con.read_until(patterns[1:], timeout=60)
        con.drain(1)
        return
    raise TimeoutError("wait_boot timeout: no shell prompt and no activate banner")


def sync_prompt(con: Console, timeout=30):
    """Abort any partial/interactive command (Ctrl+C) and resync to a clean prompt."""
    con.buf = b""
    con.send_raw("\x03")          # Ctrl+C: kill stuck interactive cmds (e.g. passwd)
    con.send_raw("\r\n")
    con.read_until([r"root@[^#]*#", r"bash[^#]*#", r"/ #", r"[#$] $"], timeout=timeout)
    con.drain(1)


def run(con: Console, cmd, expect=r"[#$] $", timeout=60):
    """Send a command and wait for the next prompt; return output."""
    con.buf = b""
    con.send(cmd)
    _, text = con.read_until(expect, timeout=timeout)
    return text


def set_password(con: Console, user, password, timeout=60):
    con.buf = b""
    con.send(f"passwd {user}" if user != "root" else "passwd")
    con.read_until(["New password", "new password"], timeout=timeout)
    con.send(password)
    con.read_until(["Retype password", "Repeat password", "retype"], timeout=timeout)
    con.send(password)
    _, text = con.read_until([r"[#$] $"], timeout=timeout)
    return text


def ssh_exec(host, cmd, user="root", password="123456", port=22, timeout=60):
    cli = paramiko.SSHClient()
    cli.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    cli.connect(host, port=port, username=user, password=password,
                timeout=20, banner_timeout=20, auth_timeout=20,
                allow_agent=False, look_for_keys=False)
    stdin, stdout, stderr = cli.exec_command(cmd, timeout=timeout)
    out = stdout.read().decode("utf-8", "replace")
    err = stderr.read().decode("utf-8", "replace")
    rc = stdout.channel.recv_exit_status()
    cli.close()
    return rc, out, err


def ssh_wait(host, user="root", password="123456", port=22, timeout=300):
    deadline = time.time() + timeout
    last = None
    while time.time() < deadline:
        try:
            return ssh_exec(host, "echo SSH_OK", user, password, port)
        except Exception as e:  # noqa: BLE001
            last = e
            time.sleep(5)
    raise TimeoutError(f"ssh_wait {host} timeout: {last}")


if __name__ == "__main__":
    import sys
    if sys.argv[1] == "console":
        c = Console(int(sys.argv[2]))
        wait_boot(c)
        print(run(c, "uname -a"))
    elif sys.argv[1] == "ssh":
        print(ssh_exec(sys.argv[2], sys.argv[3]))
