#!/bin/bash
set -euo pipefail
mkdir -p .output/codex
echo "memory-v2 evaluation (真实 PostgreSQL 与百炼配置)" | tee .output/codex/task7-eval.md
if [[ -f .env ]]; then set -a; source .env; set +a; fi
cd server
export XA_MEMORY_EVAL=1
go test -tags=integration -count=1 -timeout=40m ./internal/bootstrap -run '^TestMemoryV2$' -v 2>&1 | tee -a ../.output/codex/task7-eval.md
