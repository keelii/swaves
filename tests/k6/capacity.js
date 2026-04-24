/**
 * Swaves — 单机容量探测脚本
 * 用途：找出服务在错误率/延迟恶化之前能承受的最大并发量（破坏点测试）
 *
 * 使用方式：
 *   k6 run --env BASE_URL=http://localhost:8080 tests/k6/capacity.js
 *
 * 可选环境变量：
 *   BASE_URL   目标服务地址，默认 http://localhost:8080
 *   POST_SLUG  一篇已发布文章的 slug，用于 post 路由采样；留空则跳过 post 检查
 *   TAG_SLUG   一个已存在 tag 的 slug，用于 tag detail 路由采样；留空则跳过
 *
 * 测试阶段（ramping-vus 模式）：
 *   0 → 50 VU   2 min  预热，确认服务可用
 *   50 → 200 VU 5 min  正常压力区间
 *   200 → 500 VU 5 min 高压区间
 *   500 → 800 VU 3 min 超高压区间（多数单机会在此处出现明显降级）
 *   800 → 0 VU  2 min  冷却
 *
 * 判断依据（thresholds）：
 *   - 错误率 < 1%（http_req_failed rate < 0.01）
 *   - 95 分位响应时间 < 2000ms（读请求）
 *   超过任一阈值，k6 以非零退出码报告，CI/脚本可据此判断破坏点所在阶段。
 *
 * 查看实际破坏点：
 *   在输出的 EXECUTION 段观察 VU 数量随时间的变化，与 http_req_failed 对照，
 *   可定位到哪个并发量级开始出现大量错误或延迟飙升。
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

// ── 自定义指标 ────────────────────────────────────────────────────────────────
const errorRate    = new Rate('error_rate');
const readLatency  = new Trend('read_latency_ms', true);

// ── 配置 ──────────────────────────────────────────────────────────────────────
const BASE_URL  = __ENV.BASE_URL  || 'http://localhost:8080';
const POST_SLUG = __ENV.POST_SLUG || '';
const TAG_SLUG  = __ENV.TAG_SLUG  || '';

export const options = {
  scenarios: {
    capacity_ramp: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '2m',  target: 50  },  // 预热
        { duration: '5m',  target: 200 },  // 正常压力
        { duration: '5m',  target: 500 },  // 高压
        { duration: '3m',  target: 800 },  // 超高压 / 破坏点探测
        { duration: '2m',  target: 0   },  // 冷却
      ],
      gracefulRampDown: '30s',
    },
  },

  thresholds: {
    // 错误率不超过 1%
    'http_req_failed':                  [{ threshold: 'rate < 0.01', abortOnFail: false }],
    // 读请求 95 分位 < 2000ms
    'http_req_duration{type:read}':     [{ threshold: 'p(95) < 2000', abortOnFail: false }],
    // 自定义错误率不超过 1%
    'error_rate':                       [{ threshold: 'rate < 0.01', abortOnFail: false }],
  },
};

// ── 请求权重（模拟真实流量分布） ──────────────────────────────────────────────
// home : tags : post = 5 : 2 : 3（有 slug 时）
function pickRoute() {
  const r = Math.random();
  if (POST_SLUG && r < 0.30) return 'post';
  if (r < (POST_SLUG ? 0.50 : 0.40)) return 'tags';
  return 'home';
}

// ── 主循环 ────────────────────────────────────────────────────────────────────
export default function () {
  const route = pickRoute();
  let url;
  let expectedStatus;

  switch (route) {
    case 'post':
      url            = `${BASE_URL}/${POST_SLUG}`;
      expectedStatus = [200, 404];
      break;
    case 'tags':
      url            = TAG_SLUG
        ? `${BASE_URL}/tags/${TAG_SLUG}`
        : `${BASE_URL}/tags`;
      expectedStatus = [200];
      break;
    default: // home
      url            = `${BASE_URL}/`;
      expectedStatus = [200];
  }

  const res = http.get(url, {
    tags: { type: 'read', route },
    timeout: '10s',
  });

  const ok = expectedStatus.includes(res.status);

  check(res, {
    [`${route} ${expectedStatus.join(' or ')}`]: () => ok,
  });

  errorRate.add(!ok);
  readLatency.add(res.timings.duration);

  // 轻微随机思考时间，避免完全 CPU-bound 的人工场景
  sleep(Math.random() * 0.3 + 0.1);
}

// ── 收尾汇报 ──────────────────────────────────────────────────────────────────
export function handleSummary(data) {
  // 找到错误率开始超阈值的第一个时刻（粗粒度）
  const failed      = data.metrics.http_req_failed;
  const p95         = data.metrics['http_req_duration{type:read}'];
  const failRate    = failed  ? (failed.values.rate * 100).toFixed(2)  : 'n/a';
  const p95ms       = p95     ? p95.values['p(95)'].toFixed(2)         : 'n/a';

  const lines = [
    '═══════════════════════════════════════════════',
    '  Swaves 单机容量探测结果摘要',
    '═══════════════════════════════════════════════',
    `  http_req_failed rate  : ${failRate}%  (阈值 < 1%)`,
    `  p(95) 读请求延迟       : ${p95ms} ms  (阈值 < 2000ms)`,
    '',
    '  ▶ 查看破坏点：',
    '    在上方 EXECUTION 段观察 vus 数量随时间的变化，',
    '    结合 http_req_failed 曲线找出错误率开始明显升高时的 VU 数，',
    '    该 VU 数即为当前单机的并发承载上限参考值。',
    '═══════════════════════════════════════════════',
  ];

  console.log(lines.join('\n'));

  // 同时输出标准 k6 JSON 摘要到文件（方便 CI 归档）
  return {
    'stdout': '\n' + lines.join('\n') + '\n',
    'tests/k6/capacity-summary.json': JSON.stringify(data, null, 2),
  };
}
