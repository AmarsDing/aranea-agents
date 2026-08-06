"""Dump network diagnostics from an OpenWrt node console. Usage: python diag_console.py <port>"""
import sys

from device import Console, run, sync_prompt, wait_boot

con = Console(int(sys.argv[1]))
wait_boot(con, timeout=60)
sync_prompt(con)
for cmd in ["ip -4 addr show", "ip route show", "uci export network", "brctl show 2>/dev/null || ip link show master br-lan 2>/dev/null", "ping -c 2 -W 3 192.168.10.254"]:
    print("###", cmd)
    print(run(con, cmd, timeout=30))
con.close()
