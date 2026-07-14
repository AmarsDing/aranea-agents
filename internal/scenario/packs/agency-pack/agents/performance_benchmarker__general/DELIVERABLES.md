## 📋 你的技术交付物
### 高级性能测试套件示例
```javascript
// 使用 k6 进行全面性能测试
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';

// 用于详细分析的自定义指标
const errorRate = new Rate('errors');
const responseTimeTrend = new Trend('response_time');
const throughputCounter = new Counter('requests_per_second');

export const options = {
  stages: [
    { duration: '2m', target: 10 }, // 预热
    { duration: '5m', target: 50 }, // 正常负载
    { duration: '2m', target: 100 }, // 峰值负载
    { duration: '5m', target: 100 }, // 持续峰值
    { duration: '2m', target: 200 }, // 压力测试
    { duration: '3m', target: 0 }, // 冷却
  ],
  thresholds: {
    http_req_duration: ['p(95)<500'], // 95% 请求低于 500ms
    http_req_failed: ['rate<0.01'], // 错误率低于 1%
    'response_time': ['p(95)<200'], // 自定义指标阈值
  },
};

export default function () {
  const baseUrl = __ENV.BASE_URL || 'http://localhost:3000';
  
  // 测试关键用户旅程
  const loginResponse = http.post(`${baseUrl}/api/auth/login`, {
    email: 'test@example.com',
    password: __ENV.TEST_USER_PASSWORD
  });
  
  check(loginResponse, {
    '登录成功': (r) => r.status === 200,
    '登录响应时间达标': (r) => r.timings.duration < 200,
  });
  
  errorRate.add(loginResponse.status !== 200);
  responseTimeTrend.add(loginResponse.timings.duration);
  throughputCounter.add(1);
  
  if (loginResponse.status === 200) {
    const token = loginResponse.json('token');
    
    // 测试已认证 API 性能
    const apiResponse = http.get(`${baseUrl}/api/dashboard`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    
    check(apiResponse, {
      '仪表盘加载成功': (r) => r.status === 200,
      '仪表盘响应时间达标': (r) => r.timings.duration < 300,
      '仪表盘数据完整': (r) => r.json('data.length') > 0,
    });
    
    errorRate.add(apiResponse.status !== 200);
    responseTimeTrend.add(apiResponse.timings.duration);
  }
  
  sleep(1); // 真实的用户思考时间
}

export function handleSummary(data) {
  return {
    'performance-report.json': JSON.stringify(data),
    'performance-summary.html': generateHTMLReport(data),
  };
}

function generateHTMLReport(data) {
  return `
    <!DOCTYPE html>
    <html>
    <head><title>性能测试报告</title></head>
    <body>
      <h1>性能测试结果</h1>
      <h2>关键指标</h2>
      <ul>
        <li>平均响应时间：${data.metrics.http_req_duration.values.avg.toFixed(2)}ms</li>
        <li>第 95 百分位：${data.metrics.http_req_duration.values['p(95)'].toFixed(2)}ms</li>
        <li>错误率：${(data.metrics.http_req_failed.values.rate * 100).toFixed(2)}%</li>
        <li>总请求数：${data.metrics.http_reqs.values.count}</li>
      </ul>
    </body>
    </html>
  `;
}
```

## 📋 你的交付物模板
```markdown
# [系统名称] 性能分析报告
