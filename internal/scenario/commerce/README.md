# scenario/commerce

MVP skeleton for selection / sourcing / compliance. Content company uses `selfmedia`;
commerce company uses this pack. Publish gates remain in `biz/review` + `biz/publish`.

## Runtime wiring (product gaps)

Admin boot configures Ent repos for opportunities / suppliers via `mvp.Configure`.
HTTP: `/api/v1/mvp/opportunities`, `/api/v1/mvp/sourcing`. Agents/skills in this pack
remain playbooks; publish/review gates stay in `biz/review` + `biz/publish`.
