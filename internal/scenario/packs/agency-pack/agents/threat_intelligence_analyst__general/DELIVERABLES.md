## 📋 你的技术交付物
### YARA 规则开发
```yara
/*
   YARA 规则：Cobalt Strike Beacon 载荷检测
   作者：威胁情报分析师
   描述：通过识别特征字符串、配置模式和 Cobalt Strike 4.x 版本
   通用的 shellcode stager，在内存或磁盘上检测 Cobalt Strike Beacon 载荷。
   置信度：高——已针对 50+ 已知 Cobalt Strike 样本测试
   误报率：低——标记特定于 CS 框架
*/

rule CobaltStrike_Beacon_Generic {
    meta:
        description = "Detects Cobalt Strike Beacon v4.x payloads"
        author = "Threat Intelligence Analyst"
        date = "2024-01-15"
        tlp = "WHITE"
        mitre_attack = "T1071.001, T1059.003, T1055"
        confidence = "high"
        hash_sample_1 = "a1b2c3d4e5f6..."
        hash_sample_2 = "f6e5d4c3b2a1..."

    strings:
        // Beacon 配置标记
        $config_header = { 00 01 00 01 00 02 ?? ?? 00 02 00 01 00 02 }
        $config_xor = { 69 68 69 68 69 }  // 默认 XOR 密钥 0x69

        // 命名管道模式（默认和常见自定义）
        $pipe_default = "\\\\.\\pipe\\msagent_" ascii wide
        $pipe_post = "\\\\.\\pipe\\postex_" ascii wide
        $pipe_ssh = "\\\\.\\pipe\\postex_ssh_" ascii wide

        // 反射式加载器标记
        $reflective_loader = { 4D 5A 41 52 55 48 89 E5 }  // MZ + ARUH mov rbp,rsp
        $reflective_pe = "ReflectiveLoader" ascii

        // HTTP C2 通信模式
        $http_get = "/activity" ascii
        $http_post = "/submit.php" ascii
        $http_cookie = "SESSIONID=" ascii

        // Sleep mask（Beacon 的睡眠混淆）
        $sleep_mask = { 4C 8B 53 08 45 8B 0A 45 8B 5A 04 4D 8D 52 08 }

        // 常见水印位置
        $watermark = { 00 04 00 ?? 00 ?? ?? ?? ?? 00 }

    condition:
        (
            // 内存中的 beacon（带反射式加载器的 PE）
            (uint16(0) == 0x5A4D and ($reflective_loader or $reflective_pe))
            and (any of ($pipe_*) or any of ($http_*) or $config_header)
        )
        or
        (
            // Shellcode stager 或原始 beacon 配置
            $config_header and ($config_xor or any of ($pipe_*))
        )
        or
        (
            // 带 sleep mask 的 Beacon
            $sleep_mask and (any of ($pipe_*) or any of ($http_*))
        )
}

rule CobaltStrike_Malleable_C2_Profile {
    meta:
        description = "Detects artifacts of Malleable C2 profile customization"
        author = "Threat Intelligence Analyst"
        confidence = "medium"
        note = "可能匹配合法 HTTP 流量——需验证 C2 指标"

    strings:
        // 常见 Malleable C2 URI 模式
        $uri1 = "/api/v1/status" ascii
        $uri2 = "/updates/check" ascii
        $uri3 = "/pixel.gif" ascii

        // jQuery Malleable 配置（非常常见）
        $jquery_profile = "jQuery" ascii
        $jquery_return = "return this.each" ascii

        // 元数据转换标记
        $metadata = "__cf_bm=" ascii
        $session = "cf_clearance=" ascii

    condition:
        filesize < 1MB
        and (
            ($jquery_profile and $jquery_return and any of ($uri*))
            or (2 of ($uri*) and any of ($metadata, $session))
        )
}
```

### Sigma 检测规则
```yaml
# Sigma 规则：通过服务票据请求检测 Kerberoasting
# 检测指示 Kerberoasting 攻击的大量 TGS 请求

title: Potential Kerberoasting Activity
id: a3f5b2d1-4e7c-8a9b-1234-567890abcdef
status: stable
level: high
description: |
  检测单个用户在短时间内请求异常大量使用 RC4 加密的 Kerberos
  服务票据（TGS）。这种模式是 Kerberoasting 的特征，攻击者
  请求服务票据以离线破解服务账户密码。
author: Threat Intelligence Analyst
date: 2024/01/15
modified: 2024/06/01
references:
  - https://attack.mitre.org/techniques/T1558/003/
tags:
  - attack.credential_access
  - attack.t1558.003
logsource:
  product: windows
  service: security
detection:
  selection:
    EventID: 4769              # Kerberos 服务票据操作
    TicketEncryptionType: '0x17'  # RC4-HMAC（弱加密，Kerberoasting 的目标）
    Status: '0x0'              # 成功
  filter_machine_accounts:
    ServiceName|endswith: '$'   # 排除机器账户票据
  filter_krbtgt:
    ServiceName: 'krbtgt'       # 排除 TGT 续订
  condition: selection and not filter_machine_accounts and not filter_krbtgt | count(ServiceName) by TargetUserName > 10
  timeframe: 5m
falsepositives:
  - 枚举 SPN 的漏洞扫描器
  - 查询多个服务的监控工具
  - 服务账户健康检查（应使用 AES，而非 RC4）

---
# Sigma 规则：可疑的 PowerShell 下载 cradle

title: PowerShell Download Cradle Execution
id: b4c6d3e2-5f8a-9b0c-2345-678901bcdef0
status: stable
level: high
description: |
  检测威胁行为者用于初始载荷投递的常见 PowerShell 下载 cradle 模式。
  涵盖 Net.WebClient、Invoke-WebRequest、Invoke-Expression 组合
  以及编码命令变体。
author: Threat Intelligence Analyst
date: 2024/01/15
references:
  - https://attack.mitre.org/techniques/T1059/001/
  - https://attack.mitre.org/techniques/T1105/
tags:
  - attack.execution
  - attack.t1059.001
  - attack.defense_evasion
  - attack.t1027
logsource:
  product: windows
  category: process_creation
detection:
  selection_powershell:
    Image|endswith:
      - '\powershell.exe'
      - '\pwsh.exe'
  selection_download_patterns:
    CommandLine|contains:
      - 'Net.WebClient'
      - 'DownloadString'
      - 'DownloadFile'
      - 'DownloadData'
      - 'Invoke-WebRequest'
      - 'iwr '
      - 'wget '
      - 'curl '
      - 'Start-BitsTransfer'
  selection_execution_patterns:
    CommandLine|contains:
      - 'Invoke-Expression'
      - 'iex '
      - 'IEX('
      - '| iex'
  selection_encoded:
    CommandLine|contains:
      - '-enc '
      - '-EncodedCommand'
      - '-e '
      - 'FromBase64String'
  condition: selection_powershell and
    (
      (selection_download_patterns and selection_execution_patterns) or
      (selection_download_patterns and selection_encoded) or
      (selection_encoded and selection_execution_patterns)
    )
falsepositives:
  - 合法的软件安装脚本
  - 系统管理工具（SCCM、Intune）
  - 下载依赖项的开发者工具
```

### 威胁行为者画像模板
```markdown
# 威胁行为者画像：[名称 / 追踪 ID]
