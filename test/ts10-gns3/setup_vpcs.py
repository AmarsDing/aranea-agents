"""Configure VPCS PCs via their GNS3 telnet consoles.

VPCS console commands: ip <addr>/<prefix> <gateway> ; save
Usage: python setup_vpcs.py <pc1_port> <pc2_port>
"""
import sys
from device import Console


def config_pc(port, ip, gw="192.168.10.1", name="PC"):
    con = Console(port, timeout=60)
    con.send_raw("\r\n")
    con.drain(1)
    con.send(f"ip {ip}/24 {gw}")
    con.drain(1)
    con.send("save")
    out = con.drain(2)
    con.send("show ip")
    out += con.drain(2)
    con.close()
    print(f"[{name}] configured {ip}/24 gw {gw}")
    print(out[-300:])


if __name__ == "__main__":
    config_pc(int(sys.argv[1]), "192.168.10.10", name="PC1")
    config_pc(int(sys.argv[2]), "192.168.10.11", name="PC2")
