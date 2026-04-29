const fs = require("fs");
const path = "./public/assets/config/runtime-config.json";

const backendUrl = process.env.npm_config_backend;

if (!backendUrl) {
  console.error("缺少 --backend 参数");
  console.error("用法: npm run serve --backend=http://127.0.0.1:8080");
  process.exit(1);
}

const config = { backendUrl };
fs.writeFileSync(path, JSON.stringify(config, null, 2));
console.log(`已注入 backendUrl: ${backendUrl}`);
