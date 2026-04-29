const fs = require("fs");
const path = "./public/assets/config/runtime-config.json";

const config = { backendUrl: "" };
fs.writeFileSync(path, JSON.stringify(config, null, 2));
console.log("已清空 runtime-config backendUrl");
