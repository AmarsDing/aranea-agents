# Aranea 移动端公网接入（frp + Caddy）

手机 App 通过 `https://aranea.example.com` 访问家里/办公室 PC 上的 Aranea 后端，PC 无需公网 IP、无需放行入站端口。

```
手机 App ──HTTPS──► VPS (Caddy :443 → frps :8080) ──frp 隧道──► PC (frpc → 127.0.0.1:8800)
```

## 前置条件

| 项 | 说明 |
|---|------|
| VPS | 有公网 IP，放行 80/443（Caddy）与 7000（frps 控制面） |
| 域名 | `aranea.example.com` A 记录指向 VPS |
| frp | >= 0.52，VPS 与 PC 各一份（<https://github.com/fatedier/frp/releases>） |
| Caddy | VPS 上安装（<https://caddyserver.com/docs/install>） |

## 部署步骤

### 1. VPS：frps + Caddy

```bash
# 生成长随机 token（VPS 与 PC 共用）
openssl rand -hex 32

# 编辑 frps.toml：填入 token
# 启动 frps
frps -c frps.toml

# 编辑 Caddyfile：替换域名；启动 Caddy（自动签发 Let's Encrypt 证书）
caddy run --config Caddyfile
```

### 2. PC：frpc

```bash
# 编辑 frpc.toml：填 VPS IP、同一 token、域名
# 启动 frpc（仅出站连接，无需改防火墙）
frpc -c frpc.toml
```

Windows 可用任务计划程序 / `nssm` 注册为开机自启服务；Linux 用 systemd unit。

### 3. 验证链路

```bash
curl -i https://aranea.example.com/healthz
```

返回 200 即链路打通。然后在手机 App 首次启动的服务器配置页填入 `https://aranea.example.com`。

## 安全基线

- **frps token 必填**：不设 token 的 frps 等于把后端裸奔到公网
- **Caddy 仅暴露 443**：frps 的 8080 绑 loopback（模板默认），7000 仅 frpc 使用
- **登录限流**：Go 后端已对 `POST /v1/admins/login` 做 IP 限流 + 失败锁定；限流按 `X-Forwarded-For` 首跳取真实客户端 IP（Caddy 与 frp 均会透传/追加 XFF，无需额外配置）
- **务必修改默认密码**：`admin / changeme` 仅用于首次登录，暴露公网前必须改密
- WebSocket（`/v1/ws`）经同一隧道透传，frp 与 Caddy 默认支持 Upgrade，无需额外配置

## 故障排查

| 现象 | 排查 |
|------|------|
| `curl /healthz` 502 | Caddy → frps 链路：确认 frps 已启动且 vhostHTTPPort=8080 监听 loopback |
| frps 日志无路由 | 检查 frpc.toml `customDomains` 与请求 Host 是否一致 |
| frpc 连不上 | VPS 防火墙/安全组放行 7000；token 是否一致 |
| App 登录被 429 | 登录限流触发，等待锁定解除；确认不是密码错误重试过多 |
