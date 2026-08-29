FROM node:22-alpine AS builder
WORKDIR /app
COPY package.json ./
# 国内构建环境走 npmmirror（2026-08-29 实证：直连 npmjs 容器内 12min+ 装不完）；
# npm install 非 ci 是历史口径（package.json 与 package-lock 偶尔漂移），不动
RUN npm config set registry https://registry.npmmirror.com && npm install
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
# Quasar 产物在 dist/spa/（quasar.config 未设 distDir）；直接拷 /app/dist 会残留 nginx 默认欢迎页
COPY --from=builder /app/dist/spa /usr/share/nginx/html
EXPOSE 80
