#!/usr/bin/env bash
# ① IC M0 采集器健康检查（docs/systems/01-IC/04-开发进度.md M0 判据）
# 用法：bash apps/server/scripts/ic_collectors_verify.sh
# 端口与 app/admin/config/ic.yaml 的 base_url 对齐；任一失败即非零退出（M0 未达）。
set -u

fail=0
check() {
  local name="$1" url="$2"
  if curl -fsS -m 8 "$url" > /dev/null 2>&1; then
    echo "[OK]   $name  $url"
  else
    echo "[FAIL] $name  $url"
    fail=1
  fi
}

check newsnow  "http://127.0.0.1:6688/api/s?id=weibo"
check rsshub   "http://127.0.0.1:1200/"
check crawl4ai "http://127.0.0.1:8787/health"

if [ "$fail" -ne 0 ]; then
  echo "M0 未就绪：请先 docker compose -f infra/docker-compose.yml -f infra/ic-collectors.compose.yml up -d"
  exit 1
fi
echo "M0 采集器全部可用"
