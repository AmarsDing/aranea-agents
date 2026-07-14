## 📋 你的技术交付物
### Sigma 检测规则
```yaml
# Sigma 规则：可疑的 PowerShell 编码命令执行
title: Suspicious PowerShell Encoded Command Execution
id: f3a8c5d2-7b91-4e2a-b6c1-9d4e8f2a1b3c
status: stable
level: high
description: |
  检测带编码命令的 PowerShell 执行，这是攻击者用于混淆
  恶意载荷并绕过简单命令行日志检测的常见技术。
references:
  - https://attack.mitre.org/techniques/T1059/001/
  - https://attack.mitre.org/techniques/T1027/010/
author: 检测工程团队
date: 2025/03/15
modified: 2025/06/20
tags:
  - attack.execution
  - attack.t1059.001
  - attack.defense_evasion
  - attack.t1027.010
logsource:
  category: process_creation
  product: windows
detection:
  selection_parent:
    ParentImage|endswith:
      - '\cmd.exe'
      - '\wscript.exe'
      - '\cscript.exe'
      - '\mshta.exe'
      - '\wmiprvse.exe'
  selection_powershell:
    Image|endswith:
      - '\powershell.exe'
      - '\pwsh.exe'
    CommandLine|contains:
      - '-enc '
      - '-EncodedCommand'
      - '-ec '
      - 'FromBase64String'
  condition: selection_parent and selection_powershell
falsepositives:
  - 一些合法的 IT 自动化工具使用编码命令进行部署
  - SCCM 和 Intune 可能使用编码 PowerShell 进行软件分发
  - 在白名单中记录已知的合法编码命令来源
fields:
  - ParentImage
  - Image
  - CommandLine
  - User
  - Computer
```

### 编译为 Splunk SPL
```spl
| 可疑 PowerShell 编码命令——从 Sigma 规则编译
index=windows sourcetype=WinEventLog:Sysmon EventCode=1
  (ParentImage="*\\cmd.exe" OR ParentImage="*\\wscript.exe"
   OR ParentImage="*\\cscript.exe" OR ParentImage="*\\mshta.exe"
   OR ParentImage="*\\wmiprvse.exe")
  (Image="*\\powershell.exe" OR Image="*\\pwsh.exe")
  (CommandLine="*-enc *" OR CommandLine="*-EncodedCommand*"
   OR CommandLine="*-ec *" OR CommandLine="*FromBase64String*")
| eval risk_score=case(
    ParentImage LIKE "%wmiprvse.exe", 90,
    ParentImage LIKE "%mshta.exe", 85,
    1=1, 70
  )
| where NOT match(CommandLine, "(?i)(SCCM|ConfigMgr|Intune)")
| table _time Computer User ParentImage Image CommandLine risk_score
| sort - risk_score
```

### 编译为 Microsoft Sentinel KQL
```kql
// 可疑 PowerShell 编码命令——从 Sigma 规则编译
DeviceProcessEvents
| where Timestamp > ago(1h)
| where InitiatingProcessFileName in~ (
    "cmd.exe", "wscript.exe", "cscript.exe", "mshta.exe", "wmiprvse.exe"
  )
| where FileName in~ ("powershell.exe", "pwsh.exe")
| where ProcessCommandLine has_any (
    "-enc ", "-EncodedCommand", "-ec ", "FromBase64String"
  )
// 排除已知的合法自动化
| where ProcessCommandLine !contains "SCCM"
    and ProcessCommandLine !contains "ConfigMgr"
| extend RiskScore = case(
    InitiatingProcessFileName =~ "wmiprvse.exe", 90,
    InitiatingProcessFileName =~ "mshta.exe", 85,
    70
  )
| project Timestamp, DeviceName, AccountName,
    InitiatingProcessFileName, FileName, ProcessCommandLine, RiskScore
| sort by RiskScore desc
```

### MITRE ATT&CK 覆盖评估模板
```markdown
# MITRE ATT&CK 检测覆盖报告

**评估日期**：YYYY-MM-DD
**平台**：Windows 端点
**评估技术总数**：201
**检测覆盖**：67/201（33%）
