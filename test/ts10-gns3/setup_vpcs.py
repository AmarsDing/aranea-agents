"""Configure VPCS end hosts (PC1/PC2) via GNS3 telnet console.

VPCS console prompt is 'PCx>'. Commands:
  ip <addr>/<prefix> <gateway>   set static IP + gateway
  show ip                        verify
  save                           persist to vpcs startup config

Usage: python setup_vpcs.py <console_port> <addr/prefix> <gateway>
"""
import socket
import sys
import time

IAC = bytes([255])


def _strip_iac(data: bytes) -> bytes:
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


def rpc(sock, cmd, wait=1.5):
    sock.sendall(cmd.encode() + b"\r\n")
    time.sleep(wait)
    buf = b""
    sock.settimeout(1.0)
    try:
        while True:
            chunk = sock.recv(4096)
            if not chunk:
                break
            buf += _strip_iac(chunk)
    except socket.timeout:
        pass
    return buf.decode("utf-8", "replace")


def main(port, addr, gw):
    sock = socket.create_connection(("127.0.0.1", port), timeout=15)
    print(rpc(sock, ""))                      # wake up, get prompt
    print("[cfg] ip", addr, "gw", gw)
    print(rpc(sock, f"ip {addr} {gw}"))
    out = rpc(sock, "show ip")
    print(out)
    rpc(sock, "save")
    sock.close()
    ok = addr.split("/")[0] in out and gw in out
    print("== VPCS config", "OK ==" if ok else "CHECK FAILED ==")
    sys.exit(0 if ok else 1)


if __name__ == "__main__":
    main(int(sys.argv[1]), sys.argv[2], sys.argv[3])
