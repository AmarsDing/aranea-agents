"""First-boot provisioning of the OpenWrt edge-router via GNS3 telnet console.

Steps (all through the real device serial console):
  1. wait boot, activate console
  2. set root password to 123456
  3. create admin user (uid 1000) with password 123456
  4. LAN: 192.168.10.1/24 on br-lan(eth0); WAN: DHCP on eth1 (to GNS3 NAT)
  5. dropbear SSH: allow root + admin password login on all interfaces
  6. syslog: remote to host 192.168.10.254:5150
  7. snmpd: install via opkg (through NAT), community public, enable+start
  8. save config evidence

Usage: python setup_openwrt.py <console_port>
"""
import json
import sys
import time

from device import Console, run, set_password, sync_prompt, wait_boot, ssh_exec, ssh_wait

HOST = "192.168.10.254"
LAN_IP = "192.168.10.1"
EVIDENCE = r"F:\aranea-agents\test\ts10-gns3\evidence"


def main(port):
    con = Console(port)
    print("[console] connected, waiting for boot (TCG first boot can take minutes)...")
    wait_boot(con, timeout=600)
    print("[console] boot complete")
    sync_prompt(con)

    print(run(con, "cat /etc/openwrt_release | head -3"))

    # --- passwords ---
    print("[cfg] root password")
    print(set_password(con, "root", "123456"))
    # admin user (locked shell check: use /bin/ash, group root)
    print("[cfg] admin user")
    run(con, "grep -q '^admin:' /etc/passwd || echo 'admin:x:1000:1000:admin:/home/admin:/bin/ash' >> /etc/passwd")
    run(con, "mkdir -p /home/admin && chown admin /home/admin 2>/dev/null; true")
    print(set_password(con, "admin", "123456"))

    # --- network ---
    print("[cfg] network lan=192.168.10.1/24, wan=dhcp on eth1")
    run(con, "uci set network.lan.ipaddr='192.168.10.1'")
    run(con, "uci set network.lan.netmask='255.255.255.0'")
    # 25.12 default has NO wan section: `uci set network.wan.proto` fails unless created first
    run(con, "uci set network.wan=interface")
    run(con, "uci set network.wan.proto='dhcp'")
    run(con, "uci set network.wan.device='eth1'")
    run(con, "uci commit network")
    print(run(con, "/etc/init.d/network restart", timeout=90))
    time.sleep(5)

    # --- dropbear: listen on all, allow root pw login (default), ensure running ---
    print("[cfg] dropbear")
    run(con, "uci set dropbear.@dropbear[0].PasswordAuth='on'; uci set dropbear.@dropbear[0].RootPasswordAuth='on'; uci commit dropbear")
    run(con, "/etc/init.d/dropbear enable; /etc/init.d/dropbear restart", timeout=60)

    # --- syslog remote to host ---
    print("[cfg] syslog -> host:5150")
    run(con, f"uci set system.@system[0].log_ip='{HOST}'; uci set system.@system[0].log_port='5150'; uci set system.@system[0].log_proto='udp'; uci commit system")
    run(con, "/etc/init.d/log restart; /etc/init.d/system reload 2>/dev/null; true", timeout=60)

    # --- snmpd via apk (OpenWrt 25.x replaced opkg with apk; needs WAN) ---
    print("[cfg] apk update + add snmpd (via WAN, may take a while)...")
    out = run(con, "apk update", timeout=300)
    print(out[-800:])
    out = run(con, "apk add snmpd", timeout=600)
    print(out[-800:])
    run(con, "uci set snmpd.@agent[0].agentaddress='UDP:161' 2>/dev/null; true")
    run(con, "uci set snmpd.public=community 2>/dev/null; uci set snmpd.public.name='public' 2>/dev/null; uci set snmpd.public.source='default' 2>/dev/null; uci set snmpd.public.community='public' 2>/dev/null; uci commit snmpd 2>/dev/null; true")
    run(con, "/etc/init.d/snmpd enable; /etc/init.d/snmpd restart 2>/dev/null; true", timeout=90)

    # --- save evidence ---
    ev = {}
    ev["openwrt_release"] = run(con, "cat /etc/openwrt_release")
    ev["network"] = run(con, "uci export network")
    ev["dropbear"] = run(con, "uci export dropbear")
    ev["ip_addr"] = run(con, "ip -4 addr show")
    ev["ip_route"] = run(con, "ip route show")
    with open(EVIDENCE + r"\openwrt-provision.json", "w", encoding="utf-8") as f:
        json.dump(ev, f, ensure_ascii=False, indent=2)
    print("[saved] openwrt-provision.json")
    con.close()

    # --- verify SSH from host through the GNS3 fabric ---
    print("[verify] waiting for SSH on", LAN_IP)
    rc, out, err = ssh_wait(LAN_IP, "root", "123456", timeout=180)
    print("[verify] ssh root OK:", out.strip())
    rc, out, err = ssh_exec(LAN_IP, "echo ADMIN_OK; id", user="admin", password="123456")
    print("[verify] ssh admin:", out.strip(), err.strip())
    print("== provisioning DONE ==")


if __name__ == "__main__":
    main(int(sys.argv[1]))
