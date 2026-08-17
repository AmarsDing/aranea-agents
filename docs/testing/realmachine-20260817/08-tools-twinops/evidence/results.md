| TOOL-01 | tools list | PASS | 114ms | code=200 count=20 |
| TOOL-02 | monitoring tool keys present | FAIL | 0ms | missing=gns3_exec,gns3_fault_inject,gns3_fault_clear,gns3_health_check,twin_alarm_get,twin_line_status |
| TOOL-03 | high-risk tools requires_confirmation | FAIL | 0ms | found=0 confirmed=False |
| TOOL-04 | tool detail (gns3_health_check) | FAIL | 20ms | code=301 |
| TOOL-05 | tool runs history | PASS | 1920ms | code=200 len=6764 |
| TOOL-06 | invocation audits | PASS | 173ms | code=200 len=2899 |
| TOOL-07 | agent bindings | FAIL | 20ms | code=301 len=0 |
| TOOL-08 | live test gns3_health_check (TwinOps conn) | FAIL | 29ms | code=301 len=0 |
| TOOL-01B | tools list (page_size=500) | PASS | 142ms | code=200 count=100 total=108 |
| TOOL-02B | monitoring tool keys present | FAIL | 0ms | missing=gns3_exec,gns3_fault_inject,gns3_fault_clear,gns3_health_check,twin_alarm_get,twin_line_status |
| TOOL-03B | high-risk tools requires_confirmation | FAIL | 0ms | found=0 reqConf=0 |
| TOOL-01C | tools list (enabled=false) | PASS | 129ms | code=200 count=45 |
| TOOL-02C | monitoring tools in disabled set | FAIL | 0ms | missing=gns3_exec,gns3_fault_inject,gns3_fault_clear,gns3_health_check,twin_alarm_get,twin_line_status |
| TOOL-03C | high-risk requires_confirmation (DB seed) | FAIL | 0ms | found=0 reqConf=0 |
| TOOL-02D | monitoring tool keys present (field=key recheck) | PASS | 0ms | 17/17 gns3+twin keys found in disabled set |
| TOOL-03D | high-risk requires_confirmation (recheck) | PASS | 0ms | fault_inject/clear/alarm_ack requiresConfirmation=true |
| TOOL-10 | enable monitoring tools (17) | FAIL | 0ms | ok=15 fail=2 |
| TOOL-11 | verify enabled state | FAIL | 95ms | enabled=15/17 |
| TOOL-08C | live test gns3_health_check | FAIL | 165ms | code=400 len=133 |
| TOOL-09B | live test twin_alarm_get | FAIL | 21ms | code=301 len=0 |
| TOOL-12 | live test twin_line_status | FAIL | 21ms | code=301 len=0 |
| TOOL-04C | tool detail (gns3_health_check) | PASS | 162ms | code=200 |
| TOOL-07C | agent bindings | PASS | 248ms | code=200 len=77544 |
| TOOL-10B | enable fault_inject/clear with I_UNDERSTAND_RISK | PASS | 500ms | both 200 |
| TOOL-11B | verify enabled 17/17 | PASS | 95ms | enabled=17/17 |
