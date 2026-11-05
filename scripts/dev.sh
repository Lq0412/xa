#!/usr/bin/env bash
# 一键启动、停止和查看本地开发进程。日志与 PID 都放在 .output/。
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

log_dir="$PWD/.output/logs"
pid_dir="$PWD/.output/pids"
# 前端首次编译可能超过一分钟，健康检查留出余量。
health_timeout=300

usage() {
  printf '%s\n' '用法：bash scripts/dev.sh up|down|status' >&2
  exit 1
}

require_cmd() {
  if ! command -v "$1" >/dev/null; then
    printf '缺少命令：%s\n' "$1" >&2
    exit 1
  fi
}

# worker 目录决定是否启动；旧 PID 文件仍要能被 down/status 认出来。
manages_worker() {
  [[ -d server/cmd/worker || -f "$pid_dir/worker.pid" ]]
}

services() {
  printf '%s\n' server
  if manages_worker; then
    printf '%s\n' worker
  fi
  printf '%s\n' web web-user
}

service_port() {
  case "$1" in
    server) printf '18080' ;;
    worker) printf '18081' ;;
    web) printf '8000' ;;
    web-user) printf '8002' ;;
    *)
      printf '未知进程：%s\n' "$1" >&2
      exit 1
      ;;
  esac
}

read_pid() {
  local file="$pid_dir/$1.pid" pid
  [[ -f "$file" ]] || return 1
  pid=$(tr -d '[:space:]' < "$file")
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  printf '%s\n' "$pid"
}

# 僵尸进程对 kill -0 仍返回成功，不能当成还在提供服务。
process_alive() {
  local pid=$1 stat
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  stat=$(ps -o stat= -p "$pid" 2>/dev/null | tr -d '[:space:]' || true)
  [[ -n "$stat" && "$stat" != Z* ]]
}

port_listening() {
  local port=$1 output
  if ! output=$(ss -H -ltn "sport = :${port}"); then
    printf 'ss 检查端口 %s 失败\n' "$port" >&2
    exit 1
  fi
  [[ -n "${output//[[:space:]]/}" ]]
}

# ss -p 在非 root 下经常看不到 IPv6 监听进程，命令行里的仓库路径可以补上本项目的 node / go 进程。
repo_pids_for_port() {
  local port=$1 pid cmd root=$PWD pid_path
  for pid_path in /proc/[0-9]*; do
    pid=${pid_path##*/}
    [[ -r "$pid_path/cmdline" ]] || continue
    cmd=$(tr '\0' ' ' < "$pid_path/cmdline" 2>/dev/null || true)
    [[ -n "$cmd" ]] || continue
    case "$port" in
      18080)
        [[ "$cmd" == *"$root/.output/bin/server"* || "$cmd" == *"scripts/run.sh server"* ]] || continue
        ;;
      18081)
        [[ "$cmd" == *"$root/.output/bin/worker"* || "$cmd" == *"scripts/run.sh worker"* ]] || continue
        ;;
      8000)
        [[ "$cmd" == *"$root/web/"* && "$cmd" != *"$root/web-user/"* ]] || continue
        ;;
      8002)
        [[ "$cmd" == *"$root/web-user/"* || "$cmd" == *"PORT=8002"* ]] || continue
        ;;
      *)
        return 0
        ;;
    esac
    printf '%s\n' "$pid"
  done
}

port_pids() {
  local port=$1 output
  if ! output=$(ss -H -ltnp "sport = :${port}"); then
    printf 'ss 读取端口 %s 的进程失败\n' "$port" >&2
    exit 1
  fi
  if [[ "$output" == *pid=* ]]; then
    printf '%s\n' "$output" | grep -oE 'pid=[0-9]+' | cut -d= -f2
  fi
  repo_pids_for_port "$port"
}

kill_checked() {
  local signal=$1 target pid
  shift
  if kill -s "$signal" -- "$@" 2>/dev/null; then
    return 0
  fi
  for target in "$@"; do
    pid=${target#-}
    if [[ "$pid" =~ ^[0-9]+$ ]] && process_alive "$pid"; then
      printf '无法向进程 %s 发送信号 %s\n' "$pid" "$signal" >&2
      exit 1
    fi
  done
}

# setsid 启动的进程是会话首领，按进程组结束才能带上 npm 和 node。
signal_tree() {
  local signal=$1 pid=$2 sid child
  local -a children=()
  if ! process_alive "$pid"; then
    return 0
  fi
  if ! sid=$(ps -o sid= -p "$pid" 2>/dev/null | tr -d '[:space:]'); then
    return 0
  fi
  if [[ "$sid" == "$pid" ]]; then
    kill_checked "$signal" "-$pid"
    return 0
  fi
  readarray -t children < <(pgrep -P "$pid" || true)
  for child in "${children[@]}"; do
    [[ -n "$child" ]] || continue
    signal_tree "$signal" "$child"
  done
  if process_alive "$pid"; then
    kill_checked "$signal" "$pid"
  fi
}

stop_pid() {
  local pid=$1 i
  if [[ ! "$pid" =~ ^[0-9]+$ || "$pid" -le 1 || "$pid" -eq $$ ]]; then
    printf '拒绝停止进程 %s\n' "$pid" >&2
    exit 1
  fi
  if ! process_alive "$pid"; then
    return 0
  fi
  signal_tree TERM "$pid"
  for ((i = 0; i < 150; i++)); do
    if ! process_alive "$pid"; then
      return 0
    fi
    sleep 0.1
  done
  signal_tree KILL "$pid"
  for ((i = 0; i < 20; i++)); do
    if ! process_alive "$pid"; then
      return 0
    fi
    sleep 0.1
  done
  printf '无法停止进程 %s\n' "$pid" >&2
  exit 1
}

free_port() {
  local port=$1 i pid
  local -a pids=()
  # 进程退出后内核才会放开端口；看不到进程号时先等待，不要立刻失败。
  for ((i = 0; i < 50; i++)); do
    if ! port_listening "$port"; then
      return 0
    fi
    mapfile -t pids < <(port_pids "$port" | sort -u)
    for pid in "${pids[@]}"; do
      [[ -n "$pid" ]] || continue
      stop_pid "$pid"
    done
    sleep 0.1
  done
  if port_listening "$port"; then
    printf '端口 %s 仍在监听，无法释放\n' "$port" >&2
    exit 1
  fi
}

stop_service() {
  local name=$1 pid
  if pid=$(read_pid "$name"); then
    stop_pid "$pid"
  fi
  rm -f "$pid_dir/${name}.pid"
  free_port "$(service_port "$name")"
}

stop_all_processes() {
  local name
  while IFS= read -r name; do
    stop_service "$name"
  done < <(services)
}

log_tail() {
  local name=$1
  local log="$log_dir/${name}.log"
  printf '最近日志 %s：\n' "$log" >&2
  if [[ -f "$log" ]]; then
    tail -n 40 "$log" >&2
  else
    printf '日志文件不存在\n' >&2
  fi
}

run_logged() {
  local name=$1
  shift
  local log="$log_dir/${name}.log"
  mkdir -p "$log_dir"
  : >"$log"
  printf '开始 %s\n' "$name"
  if ! "$@" 2>&1 | tee -a "$log"; then
    printf '%s 失败\n' "$name" >&2
    log_tail "$name"
    exit 1
  fi
}

start_service() {
  local name=$1
  local log="$log_dir/${name}.log"
  local pid
  mkdir -p "$log_dir" "$pid_dir"
  printf '启动 %s %s\n' "$name" "$(date -Is)" >"$log"
  case "$name" in
    server | worker)
      setsid bash scripts/run.sh "$name" >>"$log" 2>&1 </dev/null &
      ;;
    web)
      setsid env BROWSER=none bash -c 'cd web && npm run prepare && npm run dev' >>"$log" 2>&1 </dev/null &
      ;;
    web-user)
      setsid env BROWSER=none bash -c 'cd web-user && npm run prepare && npm run dev' >>"$log" 2>&1 </dev/null &
      ;;
    *)
      printf '未知进程：%s\n' "$name" >&2
      exit 1
      ;;
  esac
  pid=$!
  if [[ ! "$pid" =~ ^[0-9]+$ ]]; then
    printf '%s 启动失败\n' "$name" >&2
    log_tail "$name"
    exit 1
  fi
  printf '%s\n' "$pid" >"$pid_dir/${name}.pid"
}

health_urls() {
  case "$1" in
    server)
      printf '%s\n' 'http://127.0.0.1:18080/health/live' 'http://127.0.0.1:18080/health/ready'
      ;;
    worker)
      printf '%s\n' 'http://127.0.0.1:18081/health/ready'
      ;;
    web)
      printf '%s\n' 'http://127.0.0.1:8000/health/ready'
      ;;
    web-user)
      printf '%s\n' 'http://127.0.0.1:8002/health/ready'
      ;;
    *)
      printf '未知进程：%s\n' "$1" >&2
      exit 1
      ;;
  esac
}

wait_health() {
  local name=$1 pid deadline url body ready last_notice
  if ! pid=$(read_pid "$name"); then
    printf '%s 没有 PID 文件\n' "$name" >&2
    exit 1
  fi
  deadline=$((SECONDS + health_timeout))
  last_notice=-15
  while ((SECONDS < deadline)); do
    if ! process_alive "$pid"; then
      printf '%s 已退出\n' "$name" >&2
      log_tail "$name"
      exit 1
    fi
    if ((SECONDS - last_notice >= 15)); then
      printf '等待 %s 健康检查\n' "$name"
      last_notice=$SECONDS
    fi
    ready=1
    while IFS= read -r url; do
      # 要求 JSON 状态，避免开发服务器尚未代理时返回 HTML 被当成成功。
      if ! body=$(curl --fail --silent --max-time 90 "$url" 2>/dev/null); then
        ready=0
        break
      fi
      if [[ "$body" != *'"status":"ok"'* ]]; then
        ready=0
        break
      fi
    done < <(health_urls "$name")
    if ((ready == 1)); then
      printf '%s 已就绪\n' "$name"
      return 0
    fi
    sleep 1
  done
  printf '%s 健康检查超时\n' "$name" >&2
  log_tail "$name"
  exit 1
}

cmd_up() {
  local name
  require_cmd ss
  require_cmd curl
  require_cmd pgrep
  require_cmd setsid
  require_cmd ps
  if [[ ! -f .env ]]; then
    printf '%s\n' '缺少 .env，先执行 make env' >&2
    exit 1
  fi
  mkdir -p "$log_dir" "$pid_dir" .output/go-tmp
  printf '%s\n' '停止旧进程'
  stop_all_processes
  run_logged deps make -C "$PWD" deps-up
  run_logged migrate make -C "$PWD" migrate
  while IFS= read -r name; do
    if [[ "$name" == "worker" && ! -d server/cmd/worker ]]; then
      continue
    fi
    printf '启动 %s\n' "$name"
    start_service "$name"
  done < <(services)
  while IFS= read -r name; do
    if [[ "$name" == "worker" && ! -d server/cmd/worker ]]; then
      continue
    fi
    wait_health "$name"
  done < <(services)
  printf '%s\n' '管理端 http://localhost:8000' '用户端 http://localhost:8002'
}

cmd_down() {
  require_cmd ss
  require_cmd pgrep
  require_cmd ps
  mkdir -p "$pid_dir"
  stop_all_processes
  # 默认保留数据库和对象存储；只有 ALL=1 时才停止 Docker 依赖。
  if [[ "${ALL:-}" == "1" ]]; then
    run_logged deps-down make -C "$PWD" deps-down
  fi
  printf '%s\n' '已停止'
}

cmd_status() {
  local name pid port state listen
  require_cmd ss
  require_cmd ps
  while IFS= read -r name; do
    port=$(service_port "$name")
    if pid=$(read_pid "$name") && process_alive "$pid"; then
      state='运行中'
    else
      state='未运行'
      pid='-'
    fi
    if port_listening "$port"; then
      listen='监听中'
    else
      listen='未监听'
    fi
    if [[ "$state" == '运行中' ]]; then
      printf '%s 运行中 pid=%s 端口 %s %s\n' "$name" "$pid" "$port" "$listen"
    else
      printf '%s 未运行 端口 %s %s\n' "$name" "$port" "$listen"
    fi
  done < <(services)
}

case "${1:-}" in
  up) cmd_up ;;
  down) cmd_down ;;
  status) cmd_status ;;
  *) usage ;;
esac
