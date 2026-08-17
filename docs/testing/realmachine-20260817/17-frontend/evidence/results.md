| FE-01 | GET / returns index.html with title | PASS | 392ms | code=200 len=720 |
| FE-02 | main JS bundle reachable | PASS | 4287ms | code=200 path=/assets/index-D-zFymAY.js size=5963207 |
| FE-03 | main CSS reachable | PASS | 582ms | code=200 path=/assets/index-sGS3016V.css size=840400 |
| FE-04 | runtime-config.json reachable & valid JSON | PASS | 14ms | code=200 content={}
 |
| FE-05 | favicon.svg reachable | PASS | 15ms | code=200 size=237 |
| FE-06 | SPA route fallback /overview | PASS | 15ms | code=200 isIndex=True |
| FE-07 | backend /healthz reachable | PASS | 27ms | code=200 |
| FE-08 | runtime-config backendUrl injection served | PASS | 15ms | served={"backendUrl":"http://localhost:8810","wsOrigin":"http://localhost:8810"} |
| FE-09 | SPA mounts & auto-redirects to /#/overview | PASS | 0ms | title ok; #q-app 1 child q-layout; nav menu rendered (zh-CN) |
| FE-10 | console error sweep after config injection | PASS | 0ms | 0 errors since navigation; pre-injection WS-to-9301 errors archived in fe10-console-preinjection.log |
| FE-11 | screenshot archive via MCP | FAIL | 0ms | browser_take_screenshot reports success but PNG never lands on disk; a11y snapshot yml used instead |
| FE-12 | /#/chat route reachable & bootstraps session | PASS | 0ms | auto-created session=6c0ec1f0 agent=agent___spirit__; app shell fully rendered |
