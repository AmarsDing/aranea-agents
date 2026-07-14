## 📋 你的技术交付物
### Cisco IOS/IOS-XE 路由器和交换机配置

```ios
! 带有用户 VLAN、OSPF 和 eBGP 边缘交接的 L3 接入交换机
vlan 20
 name USERS
!
interface Vlan20
 description Users default gateway
 ip address 10.20.0.1 255.255.255.0
 ip helper-address 10.0.0.10
 no shutdown
!
interface GigabitEthernet1/0/24
 description User access port
 switchport mode access
 switchport access vlan 20
 spanning-tree portfast
 spanning-tree bpduguard enable
!
interface GigabitEthernet0/0
 description ISP-A handoff
 ip address 203.0.113.2 255.255.255.252
 no shutdown
!
interface GigabitEthernet0/1
 description CORE-1 routed uplink
 no switchport
 ip address 10.0.0.2 255.255.255.252
 no shutdown
!
router ospf 10
 router-id 10.255.255.1
 passive-interface default
 no passive-interface GigabitEthernet0/1
 network 10.0.0.0 0.0.0.3 area 0
 network 10.20.0.0 0.0.0.255 area 0
!
ip prefix-list CUSTOMER-PREFIX seq 10 permit 198.51.100.0/24
!
route-map ISP-A-OUT permit 10
 match ip address prefix-list CUSTOMER-PREFIX
!
router bgp 65010
 bgp log-neighbor-changes
 neighbor 203.0.113.1 remote-as 65020
 neighbor 203.0.113.1 description ISP-A
 address-family ipv4
  network 198.51.100.0 mask 255.255.255.0
  neighbor 203.0.113.1 activate
  neighbor 203.0.113.1 route-map ISP-A-OUT out
 exit-address-family
```

### Cisco ASA 防火墙 NAT 和 ACL

```cisco
object network WEB-PRIVATE
 host 10.20.10.20
 nat (inside,outside) static 203.0.113.20
!
access-list OUTSIDE-IN extended permit tcp any object WEB-PRIVATE eq 443
access-list OUTSIDE-IN extended deny ip any any log
access-group OUTSIDE-IN in interface outside
!
show nat detail
show access-list OUTSIDE-IN
packet-tracer input outside tcp 198.51.100.50 54321 203.0.113.20 443 detailed
```

### Juniper Junos 路由和控制平面过滤器

```junos
set interfaces ge-0/0/0 unit 0 description ISP-A
set interfaces ge-0/0/0 unit 0 family inet address 203.0.113.2/30
set interfaces ge-0/0/1 vlan-tagging
set interfaces ge-0/0/1 unit 20 description USERS
set interfaces ge-0/0/1 unit 20 vlan-id 20
set interfaces ge-0/0/1 unit 20 family inet address 10.20.0.1/24
set interfaces ge-0/0/2 unit 0 description CORE-1
set interfaces ge-0/0/2 unit 0 family inet address 10.0.0.2/30
set protocols ospf area 0.0.0.0 interface ge-0/0/1.20 passive
set protocols ospf area 0.0.0.0 interface ge-0/0/2.0
set protocols bgp group ISP-A type external
set protocols bgp group ISP-A peer-as 65020
set protocols bgp group ISP-A neighbor 203.0.113.1
set policy-options prefix-list CUSTOMER-PREFIX 198.51.100.0/24
set policy-options policy-statement EXPORT-CUSTOMER term allow from prefix-list CUSTOMER-PREFIX
set policy-options policy-statement EXPORT-CUSTOMER term allow then accept
set policy-options policy-statement EXPORT-CUSTOMER then reject
set protocols bgp group ISP-A export EXPORT-CUSTOMER
set firewall family inet filter PROTECT-RE term allow-ssh from source-address 10.0.0.0/8
set firewall family inet filter PROTECT-RE term allow-ssh from protocol tcp
set firewall family inet filter PROTECT-RE term allow-ssh from destination-port ssh
set firewall family inet filter PROTECT-RE term allow-ssh then accept
set firewall family inet filter PROTECT-RE term drop-rest then discard
set interfaces lo0 unit 0 family inet filter input PROTECT-RE
```

### Palo Alto PAN-OS 安全策略和路由

```panos
set network interface ethernet ethernet1/1 layer3 ip 203.0.113.2/30
set network interface ethernet ethernet1/2 layer3 ip 10.20.10.1/24
set zone untrust network layer3 ethernet1/1
set zone dmz network layer3 ethernet1/2
set network virtual-router default interface ethernet1/1
set network virtual-router default interface ethernet1/2
set network virtual-router default routing-table ip static-route default-route destination 0.0.0.0/0
set network virtual-router default routing-table ip static-route default-route nexthop ip-address 203.0.113.1
set network virtual-router default routing-table ip static-route default-route interface ethernet1/1
set rulebase security rules Allow-Web from untrust to dmz source any destination 10.20.10.20 application ssl service application-default action allow
set rulebase security rules Allow-Web log-start no log-end yes
commit
```

### 故障排除命令手册

| 平台 | 基线状态 | 路由 | 交换/接口 | 防火墙/会话 |
|----------|----------------|---------|----------------------|------------------|
| Cisco IOS/IOS-XE | `show running-config`, `show version`, `show logging` | `show ip route`, `show ip ospf neighbor`, `show ip bgp summary`, `show ip cef exact-route` | `show ip interface brief`, `show interfaces status`, `show interfaces counters errors`, `show spanning-tree vlan 20` | `show access-lists`, `show control-plane host open-ports` |
| Cisco ASA/FTD CLI | `show running-config`, `show version` | `show route`, `show asp table routing` | `show interface ip brief`, `show interface` | `show conn`, `show xlate`, `show nat detail`, `packet-tracer input ... detailed` |
| Juniper Junos | `show configuration \| compare`, `show system uptime`, `show log messages` | `show route`, `show ospf neighbor`, `show bgp summary`, `show route forwarding-table` | `show interfaces terse`, `show interfaces extensive` | `show security flow session`, `show firewall filter`, `monitor traffic interface ... no-resolve` |
| Palo Alto PAN-OS | `show system info`, `show jobs all`, `show config diff` | `show routing route`, `show routing protocol bgp summary`, `test routing fib-lookup virtual-router default ip 8.8.8.8` | `show interface all`, `show counter interface all` | `show session all filter source ...`, `test security-policy-match`, `show counter global filter packet-filter yes delta yes` |

### `show` 输出解读

```text
Router# show ip bgp summary
Neighbor        V    AS MsgRcvd MsgSent TblVer InQ OutQ Up/Down  State/PfxRcd
203.0.113.1     4 65020   18231   18199    412   0    0 2d04h          24
198.51.100.5    4 65030       0       0      1   0    0 never        Active
```

解读：
- `203.0.113.1` 已建立并接收 24 个前缀。用 `show ip bgp neighbors 203.0.113.1 received-routes` 验证预期前缀计数和路由策略。
- `198.51.100.5` 卡在 `Active`，意味着 TCP 会话建立失败或被重置。检查可达性、源接口、ACL、TCP/179 和远端对等体配置。
- 健康对等体的 `InQ` 和 `OutQ` 为零，因此 BGP 没有可见的积压。

后续命令：

```ios
show ip route 198.51.100.5
show ip bgp neighbors 198.51.100.5
show tcp brief | include 198.51.100.5
show access-lists | include 179|198.51.100.5
```
