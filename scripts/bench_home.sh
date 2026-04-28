#!/usr/bin/env bash
# bench_home.sh — 用 wrk 对前台首页做基准压测，方便优化前后对比。
#
# 用法:
#   ./scripts/bench_home.sh [URL] [并发数] [线程数] [持续时间]
#
# 示例:
#   ./scripts/bench_home.sh
#   ./scripts/bench_home.sh http://127.0.0.1:4096/ 50 4 30s
#
# 输出文件（可选）:
#   设置 BENCH_OUTPUT 环境变量可把报告附加写入文件，便于前后对比：
#   BENCH_OUTPUT=bench_before.txt ./scripts/bench_home.sh
#   BENCH_OUTPUT=bench_after.txt  ./scripts/bench_home.sh

set -euo pipefail

# ---- 参数处理 ----
URL="${1:-http://127.0.0.1:4096/}"
CONCURRENCY="${2:-50}"
THREADS="${3:-4}"
DURATION="${4:-30s}"

# ---- 依赖检查 ----
if ! command -v wrk &>/dev/null; then
  echo "错误: 未找到 wrk。"
  echo "  Ubuntu/Debian: sudo apt-get install wrk"
  echo "  macOS:         brew install wrk"
  exit 1
fi

# ---- 连通性检查 ----
if ! curl -sf --max-time 3 "${URL}" -o /dev/null; then
  echo "错误: 无法连接 ${URL}"
  echo "  请确认 swaves 已启动并监听该地址，然后重试。"
  exit 1
fi

echo "========================================"
echo "  swaves 首页压测"
echo "  URL        : ${URL}"
echo "  线程数     : ${THREADS}"
echo "  并发连接数 : ${CONCURRENCY}"
echo "  持续时间   : ${DURATION}"
echo "  时间       : $(date '+%Y-%m-%d %H:%M:%S')"
echo "========================================"
echo ""

# ---- 执行压测 ----
WRK_ARGS=(
  -t "${THREADS}"
  -c "${CONCURRENCY}"
  -d "${DURATION}"
  -H "Accept-Encoding: gzip"
  "${URL}"
)

if [[ -n "${BENCH_OUTPUT:-}" ]]; then
  echo "结果同时写入: ${BENCH_OUTPUT}"
  wrk "${WRK_ARGS[@]}" | tee -a "${BENCH_OUTPUT}"
else
  wrk "${WRK_ARGS[@]}"
fi

echo ""
echo "提示："
echo "  优化前后用不同的 BENCH_OUTPUT 文件保存结果，然后对比"
echo "  'Requests/sec' 和 'Latency' 两行即可判断收益。"
echo ""
echo "  查看请求阶段耗时（启动时传入 --enable-request-timing 或设置环境变量）："
echo "  SWAVES_ENABLE_REQUEST_TIMING=1 ./swaves swaves.db"
echo "  或：./swaves --enable-request-timing swaves.db"
