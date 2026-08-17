# 19-cli CLI 测试用例

> 二进制：`bin/aranea.exe`（预构建）。配置走环境变量 ARANEA_BASE_URL / ARANEA_TOKEN，不污染用户配置。

| ID | 用例 | 预期 |
|----|------|------|
| CLI-01 | `aranea version` | 输出版本号，exit 0 |
| CLI-02 | `aranea --help` | 列出全部顶级命令 |
| CLI-03 | `aranea agent ls -o json` | exit 0，JSON 含 agent 条目 |
| CLI-04 | `aranea session ls -o json` | exit 0，JSON 会话列表 |
| CLI-05 | `aranea tool ls -o json` | exit 0，JSON 工具列表 |
| CLI-06 | `aranea monitor events -o json` | exit 0，监控事件 JSON |
| CLI-07 | `aranea session send -y --session <sid> --content <msg>` | exit 0，返回发送成功（真实 LLM 链路） |
| CLI-08 | 非法命令 `aranea nosuchcmd` | 非 0 exit + 错误提示 |
| CLI-09 | `aranea system info -o json` | exit 0，系统信息 |
