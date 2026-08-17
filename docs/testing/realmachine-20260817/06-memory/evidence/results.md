| MEM-01 | layer overview | FAIL | 30ms | code=400 len=92 |
| MEM-02 | L0 snapshots | PASS | 27ms | code=200 len=12 |
| MEM-03 | L1 tasks | PASS | 23ms | code=200 len=12 |
| MEM-04 | L3 facts list | PASS | 29ms | code=200 len=14639 |
| MEM-05 | L3 fact conflicts | FAIL | 23ms | code=400 len=106 |
| MEM-06 | L4 entities | PASS | 24ms | code=200 len=6178 |
| MEM-07 | episodes (L2) | FAIL | 24ms | code=400 len=95 |
| MEM-08 | memory worker status | PASS | 23ms | code=200 len=516 |
| MEM-09 | worker dead letters | PASS | 21ms | code=200 len=12 |
| MEM-10 | platform settings | PASS | 35ms | code=200 len=132 |
| MEM-11 | upsert L3 fact | FAIL | 37ms | code=400 fid= |
| MEM-12 | recall debug | FAIL | 26ms | code=400 len=95 |
| MEM-13 | composite search | FAIL | 26ms | code=400 len=95 |
| MEM-14 | agent identity | PASS | 22ms | code=200 len=137 |
| MEM-15 | unified memory graph | FAIL | 23ms | code=400 len=92 |
| MEM-16 | cascade proposals | FAIL | 21ms | code=400 len=95 |
| MEM-01B | layer overview (agent_id) | PASS | 68ms | code=200 len=4114 |
| MEM-05B | L3 conflicts (agent_id) | PASS | 22ms | code=200 len=23 |
| MEM-07B | episodes (agent_id) | PASS | 22ms | code=200 len=11065 |
| MEM-11B | upsert L3 fact (fact wrapper) | FAIL | 30ms | code=400 fid= |
| MEM-12B | recall debug (agent_id) | PASS | 28ms | code=200 len=5204 |
| MEM-13B | composite search (agent_id) | PASS | 29ms | code=200 len=833 |
| MEM-15B | unified graph (agent_id) | PASS | 36ms | code=200 len=650 |
| MEM-16B | cascade proposals (agent_id) | PASS | 22ms | code=200 len=12 |
| MEM-11C | upsert L3 fact (scope_type+statement) | FAIL | 309ms | code=400 fid= msg=scope_id is required for agent writes |
| MEM-11D | fact list verify | FAIL | 45ms | code=200 hit=False |
| MEM-11C | upsert L3 fact (scope_type+statement) | PASS | 321ms | code=200 fid=ce3d9bb3-69ba-4e66-aaac-801e30d812bd msg= |
| MEM-11D | fact list verify | PASS | 27ms | code=200 hit=True |
| MEM-17 | fact review (confirm) | PASS | 24ms | code=200 msg= |
