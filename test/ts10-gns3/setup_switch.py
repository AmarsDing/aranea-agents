"""First-boot provisioning of SW1 (OpenWrt as L2 access switch) via telnet console.

Config:
  1. root/admin passwords (123456)
  2. br-lan bridges eth0-eth3 (pure L2 switch), mgmt IP 192.168.10.2/24
  3. gateway/dns via edge-router 192.168.10.1 (for opkg through router NAT)
  4. dnsmasq + firewall DISABLED (switch must not serve DHCP on the LAN)
  5. dropbear SSH on, syslog remote to host 192.168.10.254:5150
  6. snmpd via opkg (through edge-router NAT), community public

Usage: python setup_switch.py <console_port>
"""
import json
import sys
import time

from device import Console, run, set_password, sync_prompt, wait_boot, ssh_exec, ssh_wait

HOST = "192.168.10.254"
LAN_IP = "192.168.10.2"
GW = "192.168.10.1"
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
    print("[cfg] admin user")
    run(con, "grep -q '^admin:' /etc/passwd || echo 'admin:x:1000:1000:admin:/home/admin:/bin/ash' >> /etc/passwd")
    run(con, "mkdir -p /home/admin && chown admin /home/admin 2>/dev/null; true")
    print(set_password(con, "admin", "123456"))

    # --- network: pure L2 switch ---
    print("[cfg] bridge eth0-eth3 as br-lan, mgmt 192.168.10.2/24 gw 192.168.10.1")
    run(con, "uci delete network.wan 2>/dev/null; uci delete network.wan6 2>/dev/null; true")
    run(con, "uci set network.@device[0].type='bridge'")
    run(con, "uci set network.@device[0].name='br-lan'")
    # ports is a UCI list on 25.12: must delete + add_list per port (space-separated string is wrong)
    run(con, "uci delete network.@device[0].ports 2>/dev/null; true")
    for p in ("eth0", "eth1", "eth2", "eth3"):
        run(con, f"uci add_list network.@device[0].ports='{p}'")
    run(con, "uci set network.lan.device='br-lan'")
    run(con, "uci set network.lan.proto='static'")
    run(con, f"uci set network.lan.ipaddr='{LAN_IP}'")
    run(con, "uci set network.lan.netmask='255.255.255.0'")
    run(con, f"uci set network.lan.gateway='{GW}'")
    run(con, f"uci set network.lan.dns='{GW}'")
    run(con, "uci commit network")
    # switch must not run DHCP/DNS or firewall on the LAN
    run(con, "/etc/init.d/dnsmasq disable; /etc/init.d/dnsmasq stop 2>/dev/null; true")
    run(con, "/etc/init.d/firewall disable; /etc/init.d/firewall stop 2>/dev/null; true")
    print(run(con, "/etc/init.d/network restart", timeout=90))
    time.sleep(5)

    # --- dropbear ---
    print("[cfg] dropbear")
    run(con, "uci set dropbear.@dropbear[0].PasswordAuth='on'; uci set dropbear.@dropbear[0].RootPasswordAuth='on'; uci commit dropbear")
    run(con, "/etc/init.d/dropbear enable; /etc/init.d/dropbear restart", timeout=60)

    # --- syslog remote to host ---
    print("[cfg] syslog -> host:5150")
    run(con, f"uci set system.@system[0].log_ip='{HOST}'; uci set system.@system[0].log_port='5150'; uci set system.@system[0].log_proto='udp'; uci commit system")
    run(con, "/etc/init.d/log restart; /etc/init.d/system reload 2>/dev/null; true", timeout=60)

    # --- snmpd via apk (OpenWrt 25.x; through edge-router NAT) ---
    print("[cfg] apk update + add snmpd (via edge-router NAT)...")
    out = run(con, "apk update", timeout=300)
    print(out[-500:])
    out = run(con, "apk add snmpd", timeout=600)
    print(out[-500:])
    run(con, "uci set snmpd.@agent[0].agentaddress='UDP:161' 2>/dev/null; true")
    run(con, "uci set snmpd.public=community 2>/dev/null; uci set snmpd.public.name='public' 2>/dev/null; uci set snmpd.public.source='default' 2>/dev/null; uci set snmpd.public.community='public' 2>/dev/null; uci commit snmpd 2>/dev/null; true")
    run(con, "/etc/init.d/snmpd enable; /etc/init.d/snmpd restart 2>/dev/null; true", timeout=90)

    # --- save evidence ---
    ev = {}
    ev["openwrt_release"] = run(con, "cat /etc/openwrt_release")
    ev["network"] = run(con, "uci export network")
    ev["dropbear"] = run(con, "uci export dropbear")
    ev["bridge"] = run(con, "brctl show 2>/dev/null || ip link show master br-lan")
    ev["ip_addr"] = run(con, "ip -4 addr show")
    with open(EVIDENCE + r"\sw1-provision.json", "w", encoding="utf-8") as f:
        json.dump(ev, f, ensure_ascii=False, indent=2)
    print("[saved] sw1-provision.json")
    con.close()

    # --- verify SSH from host through the GNS3 fabric ---
    print("[verify] waiting for SSH on", LAN_IP)
    rc, out, err = ssh_wait(LAN_IP, "root", "123456", timeout=180)
    print("[verify] ssh root OK:", out.strip())
    rc, out, err = ssh_exec(LAN_IP, "echo ADMIN_OK; id", user="admin", password="123456")
    print("[verify] ssh admin:", out.strip(), err.strip())
    print("== switch provisioning DONE ==")


if __name__ == "__main__":
    main(int(sys.argv[1]))
