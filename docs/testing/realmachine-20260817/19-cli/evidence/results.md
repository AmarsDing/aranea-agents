| CLI-01 | aranea version | PASS | 382ms | exit=0 out=█████╗ ██████╗  █████╗ ███╗   ██╗███████╗ █████╗ 
██╔══██╗█ |
| CLI-02 | aranea --help lists commands | FAIL | 107ms | exit=0 topLevelCmds~0 |
| CLI-03 | agent ls -o json | FAIL | 102ms | exit=0 agents=0 |
| CLI-04 | session ls -o json | PASS | 103ms | exit=0 sessions=0 |
| CLI-05 | tool ls -o json | FAIL | 102ms | exit=0 tools=0 |
| CLI-06 | monitor events -o json | PASS | 102ms | exit=0 len=397 |
| CLI-07 | session send -y (real LLM roundtrip) | PASS | 104ms | exit=0 sid=6c0ec1f0-e4e2-4c7f-9ea0-17fa8da779d8 out=█████╗ ██████╗  █████╗ ███╗   ██╗███████╗ █████╗ 
██╔══██╗██╔══██╗██╔══██╗████╗ |
| CLI-08 | invalid command exit!=0 with error | FAIL | 114ms | exit=0 |
| CLI-09 | system info -o json | PASS | 105ms | exit=0 len=397 |
| CLI-01 | aranea version | PASS | 439ms | exit=0 out=█████╗ ██████╗  █████╗ ███╗   ██╗███████╗ █████╗ 
██╔══██╗█ |
| CLI-02 | aranea --help lists commands | FAIL | 103ms | exit=0 topLevelCmds~0 |
| CLI-03 | agent ls -o json | FAIL | 107ms | exit=0 agents=0 |
| CLI-04 | session ls -o json | PASS | 105ms | exit=0 sessions=0 |
| CLI-05 | tool ls -o json | FAIL | 103ms | exit=0 tools=0 |
| CLI-06 | monitor events -o json | PASS | 111ms | exit=0 len=397 |
| CLI-07 | session send -y (real LLM roundtrip) | PASS | 106ms | exit=0 sid=6c0ec1f0-e4e2-4c7f-9ea0-17fa8da779d8 out=█████╗ ██████╗  █████╗ ███╗   ██╗███████╗ █████╗ 
██╔══██╗██╔══██╗██╔══██╗████╗ |
| CLI-08 | invalid command exit!=0 with error | FAIL | 103ms | exit=0 |
| CLI-09 | system info -o json | PASS | 106ms | exit=0 len=397 |
| CLI-01 | aranea version | PASS | 391ms | exit=0 out=█████╗ ██████╗  █████╗ ███╗   ██╗███████╗ █████╗ 
██╔══██╗█ |
| CLI-02 | aranea --help lists commands | FAIL | 108ms | exit=0 topLevelCmds~0 |
| CLI-03 | agent ls -o json | FAIL | 121ms | exit=0 agents=0 |
| CLI-04 | session ls -o json | PASS | 113ms | exit=0 sessions=0 |
| CLI-05 | tool ls -o json | FAIL | 106ms | exit=0 tools=0 |
| CLI-06 | monitor events -o json | PASS | 107ms | exit=0 len=397 |
| CLI-07 | session send -y (real LLM roundtrip) | PASS | 103ms | exit=0 sid=6c0ec1f0-e4e2-4c7f-9ea0-17fa8da779d8 out=█████╗ ██████╗  █████╗ ███╗   ██╗███████╗ █████╗ 
██╔══██╗██╔══██╗██╔══██╗████╗ |
| CLI-08 | invalid command exit!=0 with error | FAIL | 105ms | exit=0 |
| CLI-09 | system info -o json | PASS | 107ms | exit=0 len=397 |
| CLI-01 | aranea version | PASS | 293ms | exit=0 out=█████╗ ██████╗  █████╗ ███╗   ██╗███████╗ █████╗ 
██╔══██╗█ |
| CLI-02 | aranea --help lists commands | FAIL | 104ms | exit=0 topLevelCmds~0 |
| CLI-03 | agent ls -o json | FAIL | 109ms | exit=0 agents=0 |
| CLI-04 | session ls -o json | PASS | 108ms | exit=0 sessions=0 |
| CLI-05 | tool ls -o json | FAIL | 106ms | exit=0 tools=0 |
| CLI-06 | monitor events -o json | PASS | 103ms | exit=0 len=397 |
| CLI-07 | session send -y (real LLM roundtrip) | PASS | 103ms | exit=0 sid=6c0ec1f0-e4e2-4c7f-9ea0-17fa8da779d8 out=█████╗ ██████╗  █████╗ ███╗   ██╗███████╗ █████╗ 
██╔══██╗██╔══██╗██╔══██╗████╗ |
| CLI-08 | invalid command exit!=0 with error | FAIL | 103ms | exit=0 |
| CLI-09 | system info -o json | PASS | 103ms | exit=0 len=397 |
| CLI-01 | aranea version | PASS | 383ms | exit=0 out=cli_version               dev
commit                    non |
| CLI-02 | aranea --help lists commands | PASS | 85ms | exit=0 topLevelCmds~15 |
| CLI-03 | agent ls -o json | PASS | 113ms | exit=0 agents=20 |
| CLI-04 | session ls -o json | PASS | 109ms | exit=0 sessions=20 |
| CLI-05 | tool ls -o json | PASS | 179ms | exit=0 tools=20 |
| CLI-06 | monitor events -o json | PASS | 274ms | exit=0 len=7000 |
| CLI-07 | session send -y (real LLM roundtrip) | PASS | 8897ms | exit=0 sid=6c0ec1f0-e4e2-4c7f-9ea0-17fa8da779d8 out=✓  消息已发送
  session_id        6c0ec1f0-e4e2-4c7f-9ea0-17fa8da779d8 |
| CLI-08 | invalid command exit!=0 with error | FAIL | 99ms | exit=3 |
| CLI-09 | system info -o json | PASS | 115ms | exit=0 len=275 |
| CLI-08 | 非法命令stderr非空(BUG-CLI-01 复测) | PASS | 99ms | exit=3 stderr=aranea: unknown command foobar |
