#!/usr/bin/env bash
# 生成仅供本地开发使用的凭证，不覆盖已有配置。
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
if [[ -e .env ]]; then
  printf '%s\n' '.env 已存在，请直接使用或手动编辑。' >&2
  exit 1
fi
command -v openssl >/dev/null
pg_secret=$(openssl rand -hex 24)
redis_secret=$(openssl rand -hex 24)
s3_key=$(openssl rand -hex 12)
s3_secret=$(openssl rand -hex 24)
umask 077
set -o noclobber
{
  cat .env.example
  printf '\nXA_POSTGRES_PASSWORD=%s\nXA_REDIS_PASSWORD=%s\nXA_S3_ACCESS_KEY=%s\nXA_S3_SECRET_KEY=%s\n' \
    "$pg_secret" "$redis_secret" "$s3_key" "$s3_secret"
} > .env
printf '%s\n' '已生成 .env，凭证未输出到终端。'
