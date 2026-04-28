#!/usr/bin/env bash
# bench_home.sh — 用 ab 对前台首页做基准压测，方便优化前后对比。
#
# 用法:
#   ./scripts/bench_home.sh [URL] [并发数] [总请求数]
#
# 示例:
#   ./scripts/bench_home.sh http://localhost:3000/ 10 500
#   ./scripts/bench_home.sh https://example.com/  20 1000
#
# 输出文件（可选）:
#   设置 BENCH_OUTPUT 环境变量可把报告附加写入文件，便于前后对比：
#   BENCH_OUTPUT=bench_before.txt ./scripts/bench_home.sh http://localhost:3000/
#   BENCH_OUTPUT=bench_after.txt  ./scripts/bench_home.sh http://localhost:3000/

set -euo pipefail

# ---- 参数处理 ----
URL="${1:-http://localhost:3000/}"
CONCURRENCY="${2:-10}"
REQUESTS="${3:-200}"

# ---- 依赖检查 ----
if ! command -v ab &>/dev/null; then
  echo "错误: 未找到 ab (Apache Bench)。"
  echo "  Ubuntu/Debian: sudo apt-get install apache2-utils"
  echo "  macOS:         brew install httpd   (或系统自带 /usr/sbin/ab)"
  exit 1
fi

echo "========================================"
echo "  swaves 首页压测"
echo "  URL        : ${URL}"
echo "  并发数     : ${CONCURRENCY}"
echo "  总请求数   : ${REQUESTS}"
echo "  时间       : $(date '+%Y-%m-%d %H:%M:%S')"
echo "========================================"
echo ""

# ---- 执行压测 ----
AB_ARGS=(
  -n "${REQUESTS}"
  -c "${CONCURRENCY}"
  -q            # 安静模式，不打印进度点
  -H "Accept-Encoding: gzip"
  "${URL}"
)

if [[ -n "${BENCH_OUTPUT:-}" ]]; then
  echo "结果同时写入: ${BENCH_OUTPUT}"
  ab "${AB_ARGS[@]}" | tee -a "${BENCH_OUTPUT}"
else
  ab "${AB_ARGS[@]}"
fi

echo ""
echo "提示："
echo "  优化前后用不同的 BENCH_OUTPUT 文件保存结果，然后对比"
echo "  'Requests per second' 和 'Time per request' 两行即可判断收益。"
echo ""
echo "  查看请求阶段耗时（debug_request_timing 可热生效，无需重启）："
echo "  UPDATE t_settings SET value='1' WHERE code='debug_request_timing';"
echo "  关闭：UPDATE t_settings SET value='0' WHERE code='debug_request_timing';"
