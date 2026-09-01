#!/usr/bin/env bash
# llama灵境 启动脚本：构建前端（若缺失）+ 启动 FastAPI 单进程
set -euo pipefail
cd "$(dirname "$0")"

if [ ! -f config/hosts.yaml ]; then
  echo "错误：config/hosts.yaml 不存在。" >&2
  echo "请先执行：cp config/hosts.example.yaml config/hosts.yaml 并填写主机信息" >&2
  echo "同时：cp .env.example .env 填写 SSH 密码" >&2
  exit 1
fi

if [ ! -f frontend/dist/index.html ]; then
  echo "frontend/dist 未构建，开始构建前端（首次较慢）..."
  (cd frontend && npm install && npm run build)
fi

# --timeout-graceful-shutdown：存在半死 WS 客户端时，优雅关闭最多等 10s 后强制结束，避免进程卡在关闭阶段
exec python3 -m uvicorn backend.main:app --host 0.0.0.0 --port "${PORT:-8000}" --timeout-graceful-shutdown 10
