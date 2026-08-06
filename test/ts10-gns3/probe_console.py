"""Quick console probe: connect, send Enter, dump whatever comes back for N seconds."""
import socket
import sys
import time

port = int(sys.argv[1])
secs = int(sys.argv[2]) if len(sys.argv) > 2 else 10

s = socket.create_connection(("127.0.0.1", port), timeout=10)
s.settimeout(1.0)
buf = b""
end = time.time() + secs
if len(sys.argv) > 3 and sys.argv[3] == "poke":
    s.sendall(b"\r\n")
while time.time() < end:
    try:
        chunk = s.recv(4096)
        if chunk:
            buf += chunk
    except socket.timeout:
        pass
s.close()
# strip IAC
out = bytearray()
i = 0
while i < len(buf):
    if buf[i] == 255 and i + 1 < len(buf):
        cmd = buf[i + 1]
        if cmd in (251, 252, 253, 254) and i + 2 < len(buf):
            i += 3
            continue
        i += 2
        continue
    out.append(buf[i])
    i += 1
print(bytes(out).decode("utf-8", "replace")[-2000:])
print("---- bytes received:", len(buf))
