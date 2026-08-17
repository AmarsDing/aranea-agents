| SPIRIT-01 | spirit agent detail | PASS | 40ms | code=200 key=__spirit__ |
| SPIRIT-02 | chat options | PASS | 23ms | code=200 len=324 |
| SPIRIT-03 | create spirit session | PASS | 35ms | code=200 sid=6c0ec1f0-e4e2-4c7f-9ea0-17fa8da779d8 |
| SPIRIT-04 | spirit send msg -> LLM reply | FAIL | 9924ms | code=200 reply=False |
| SPIRIT-05 | spirit teams list | PASS | 21ms | code=200 len=12 |
| SPIRIT-06 | spirit task plans | PASS | 22ms | code=200 len=12 |
| SPIRIT-07 | synthesize (empty teams) | PASS | 22ms | code=400 msg=[SPIRIT/BAD_REQUEST] no completed or failed teams to synthesize |
| SPIRIT-04B | spirit send msg -> LLM reply (field recheck: agentMessage camelCase)
| SPIRIT-04C | spirit send msg recheck | PASS | 9924ms | reply=PONG token_in=24199 token_out=19 |
