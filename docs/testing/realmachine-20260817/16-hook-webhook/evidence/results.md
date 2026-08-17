| HK-01 | hook list | PASS | 30ms | code=200 hooks=0 |
| HK-02 | hook create | FAIL | 30ms | code=400 id= |
| HK-03 | hook get | SKIP | 0ms | create failed |
| HK-04 | hook patch | SKIP | 0ms | create failed |
| HK-05 | hook delete | SKIP | 0ms | create failed |
| HK-06 | hook deliveries | PASS | 22ms | code=200 |
| HK-01 | hook list | PASS | 27ms | code=200 hooks=0 |
| HK-02 | SSRF guard rejects private webhook | PASS | 29ms | code=400 |
| HK-02B | hook create (public url) | PASS | 119ms | code=200 id=30978e61b1a0aa0b33d3e121 |
| HK-03 | hook get | PASS | 21ms | code=200 |
| HK-04 | hook patch | PASS | 24ms | code=200 |
| HK-05 | hook delete | PASS | 21ms | code=200 |
| HK-06 | hook deliveries | PASS | 20ms | code=200 |
