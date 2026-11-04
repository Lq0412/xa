#!/usr/bin/env bash
# 使用项目环境构建并运行单个进程，exec 保证终端信号传给实际 Go 程序。
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
case "${1:-}" in
  server|worker|migrate|init-storage) process_name=$1 ;;
  *) printf '%s\n' '用法：bash scripts/run.sh server|worker|migrate|init-storage' >&2; exit 1 ;;
esac
set -a
source .env
set +a
mkdir -p .output/go-tmp .output/bin
export GOTMPDIR="$PWD/.output/go-tmp"
export TMPDIR="$GOTMPDIR"
binary_path="$PWD/.output/bin/$process_name"
cd server
go build -o "$binary_path" "./cmd/$process_name"
exec "$binary_path"
