#!/usr/bin/env bash
# Upgrade one existing Compose service, retaining its project, ports and mounts.
set -euo pipefail

usage() {
  cat <<'HELP'
Usage:
  bash scripts/upgrade.sh [--file compose.core.yml] [--service core]
  bash scripts/upgrade.sh --container EXISTING_CONTAINER
Options:
  -f, --file FILE       Existing Compose file (repeat for multiple files)
  -s, --service NAME    Application service, default: core
  -c, --container NAME Read Compose project/service/files from an existing container
  -p, --project NAME   Explicit Compose project name
      --env-file FILE  Existing Compose interpolation env file (repeatable)
      --image REF      Published image, default: ghcr.io/lqy007700/codex2apicore:latest
      --timeout SEC    Health probe deadline, default: 120
      --rollback       Use the previous local application image; no registry pull
      --dry-run        Inspect target and print actions without changing anything
  -h, --help
Run from the existing deployment directory. Never replace its volume mappings
with a fresh-install example. This script does not update source code or .env.
HELP
}
die() { printf '升级失败：%s\n' "$*" >&2; exit 1; }
info() { printf '%s\n' "$*"; }
need_value() { [[ $# -ge 2 && -n "$2" ]] || die "$1 缺少参数"; }
files=(); env_files=(); service=core; container=; project=; workdir=; rollback=false; dry_run=false
image=ghcr.io/lqy007700/codex2apicore:latest
health_timeout=120
service_explicit=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    -f|--file) need_value "$@"; files+=("$2"); shift 2 ;;
    -s|--service) need_value "$@"; service=$2; service_explicit=true; shift 2 ;;
    -c|--container) need_value "$@"; container=$2; shift 2 ;;
    -p|--project) need_value "$@"; project=$2; shift 2 ;;
    --env-file) need_value "$@"; env_files+=("$2"); shift 2 ;;
    --image) need_value "$@"; image=$2; shift 2 ;;
    --timeout) need_value "$@"; health_timeout=$2; shift 2 ;;
    --rollback) rollback=true; shift ;;
    --dry-run) dry_run=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "未知参数 $1；使用 --help 查看用法" ;;
  esac
done
[[ "$health_timeout" =~ ^[1-9][0-9]*$ && ${#health_timeout} -le 5 ]] || die '--timeout 必须为 1～86400 秒'
(( health_timeout <= 86400 )) || die '--timeout 不能超过 86400 秒'
[[ "$service" =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]] || die '服务名无效'
[[ "$image" =~ ^[a-zA-Z0-9][a-zA-Z0-9._:/@-]*$ ]] || die '镜像引用无效'
command -v docker >/dev/null || die '未找到 docker 命令'
docker compose version >/dev/null 2>&1 || die '需要 Docker Compose v2（docker compose）'
docker info >/dev/null 2>&1 || die 'Docker 不可用，请检查服务和当前用户权限'

label() { docker inspect --format "{{ index .Config.Labels \"$2\" }}" "$1"; }
if [[ -n "$container" ]]; then
  target_service=$(label "$container" com.docker.compose.service) || die '无法读取目标容器'
  target_project=$(label "$container" com.docker.compose.project)
  [[ -n "$target_service" && "$target_service" != '<no value>' && -n "$target_project" && "$target_project" != '<no value>' ]] || die '该容器没有 Compose 标签；docker run 部署请先提供原启动配置'
  if $service_explicit && [[ "$service" != "$target_service" ]]; then die '指定服务名与目标容器不一致'; fi
  service=$target_service
  [[ -z "$project" || "$project" == "$target_project" ]] || die '指定项目名与目标容器不一致'
  project=$target_project
  workdir=$(label "$container" com.docker.compose.project.working_dir)
  [[ -d "$workdir" ]] || die '原 Compose 工作目录不存在，请使用 --file 指定仍有效的部署配置'
  if [[ ${#files[@]} -eq 0 ]]; then
    config_files=$(label "$container" com.docker.compose.project.config_files)
    [[ -n "$config_files" && "$config_files" != '<no value>' ]] || die '缺少原 Compose 文件标签，请使用 --file'
    IFS=',' read -r -a files <<< "$config_files"
  fi
  if [[ ${#env_files[@]} -eq 0 ]]; then
    config_env=$(label "$container" com.docker.compose.project.environment_file)
    if [[ -n "$config_env" && "$config_env" != '<no value>' ]]; then IFS=',' read -r -a env_files <<< "$config_env"; fi
  fi
fi
[[ "$service" =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]] || die '目标服务名无效'
if [[ ${#files[@]} -eq 0 ]]; then
  [[ -f compose.core.yml ]] || die '未找到 compose.core.yml，请通过 --file/--service 或 --container 指定现有部署'
  files=(compose.core.yml)
fi
for file in "${files[@]}"; do [[ -f "$file" ]] || die "Compose 文件不存在：$file"; done
if [[ -z "$workdir" ]]; then workdir=$(cd "$(dirname "${files[0]}")" && pwd); fi
# Resolve paths before any cd. Keep the same project directory for .env and mounts.
resolved_files=()
for file in "${files[@]}"; do
  resolved="$(cd "$(dirname "$file")" && pwd)/$(basename "$file")"
  # Container labels include our last overlay after a successful recreation.
  # Do not recursively append it as another base file on subsequent upgrades.
  if [[ -n "$project" && "$resolved" == "$workdir/.codex2api-upgrade/$project/$service/active-image.json" ]]; then continue; fi
  resolved_files+=("$resolved")
done
[[ ${#resolved_files[@]} -gt 0 ]] || die '缺少原始 Compose 配置，不能只使用升级镜像覆盖文件'
files=("${resolved_files[@]}")
compose=(docker compose --project-directory "$workdir")
if [[ -n "$project" ]]; then compose+=(-p "$project"); fi
for file in "${files[@]}"; do compose+=(-f "$file"); done
if [[ ${#env_files[@]} -gt 0 ]]; then
  for file in "${env_files[@]}"; do [[ -f "$file" ]] || die "环境文件不存在：$file"; compose+=(--env-file "$file"); done
fi
services=$("${compose[@]}" config --services) || die 'Compose 配置校验失败'
found=false
while IFS= read -r name; do [[ "$name" != "$service" ]] || found=true; done <<< "$services"
$found || die "配置中没有服务 $service"
cid=$("${compose[@]}" ps --all --quiet "$service") || die '无法定位已有服务'
[[ -n "$cid" && "$cid" != *$'\n'* ]] || die '仅支持升级一个已存在的应用容器；请核对项目、服务名及副本数'
actual_project=$(label "$cid" com.docker.compose.project)
actual_service=$(label "$cid" com.docker.compose.service)
[[ "$actual_service" == "$service" && -n "$actual_project" && "$actual_project" != '<no value>' ]] || die '容器标签与目标服务不一致'
[[ -z "$project" || "$project" == "$actual_project" ]] || die '目标项目不一致'
if [[ -n "$container" ]]; then
  requested_id=$(docker inspect --format '{{.Id}}' "$container")
  actual_id=$(docker inspect --format '{{.Id}}' "$cid")
  [[ "$requested_id" == "$actual_id" ]] || die 'Compose 文件解析出的容器与 --container 目标不一致'
fi
project=$actual_project
[[ "$project" =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]] || die '项目名无效'
# Pin the existing project even when its name was originally supplied via -p.
compose+=(-p "$project")
old_image=$(docker inspect --format '{{.Image}}' "$cid")
[[ "$old_image" == sha256:* ]] || die '无法读取当前镜像 ID'
state_dir="$workdir/.codex2api-upgrade/$project/$service"
active_file="$state_dir/active-image.json"
previous_file="$state_dir/previous-image"
if $rollback; then
  [[ -f "$previous_file" ]] || die '没有此服务的历史升级镜像记录'
  image=$(cat "$previous_file")
  [[ "$image" =~ ^codex2apicore-rollback:[a-zA-Z0-9_.-]+$ ]] || die '历史镜像记录无效'
  docker image inspect "$image" >/dev/null || die '旧镜像已被删除，无法本地回退'
fi
info "目标项目：${project}；应用服务：${service}；容器：$cid"
info "目标镜像：$image"
if $dry_run; then
  info '只读预览：将拉取目标镜像（回退时不拉取），记录旧镜像，再仅重建此服务并检查 /health。'
  info "保留现有 Compose 配置、环境文件和挂载；镜像覆盖记录位于 $active_file"
  exit 0
fi
umask 077
mkdir -p "$state_dir"
# A project/service lock prevents concurrent upgrades from overwriting rollback state.
lock_dir="$state_dir/lock"
mkdir "$lock_dir" 2>/dev/null || die "已有升级操作或遗留锁：${lock_dir}；确认没有其他升级后再移除该目录"
stage_file="$state_dir/staging-$$.json"
cleanup() { rm -f "$stage_file"; rmdir "$lock_dir" 2>/dev/null || true; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
write_override() { printf '{"services":{"%s":{"image":"%s"}}}\n' "$service" "$1" > "$2"; }
write_override "$image" "$stage_file"
if ! $rollback; then
  info '正在拉取镜像，当前容器继续运行…'
  if ! "${compose[@]}" -f "$stage_file" pull "$service"; then
    die '镜像拉取失败，当前容器未替换。若 GHCR 返回 denied/unauthorized，请将自己的镜像包设为 public，或先 docker login ghcr.io'
  fi
fi
new_image=$(docker image inspect --format '{{.Id}}' "$image") || die '拉取后无法读取镜像'
if [[ "$new_image" == "$old_image" ]] && ! $rollback; then
  info '当前容器已经使用此镜像，跳过重建。'
else
  stamp=$(date -u +%Y%m%dT%H%M%SZ)-$$
  old_tag="codex2apicore-rollback:$project-$service-$stamp"
  new_tag="codex2apicore-deploy:$project-$service-$stamp"
  docker tag "$old_image" "$old_tag"
  docker tag "$new_image" "$new_tag"
  printf '%s\n' "$old_tag" > "$previous_file.tmp"
  mv "$previous_file.tmp" "$previous_file"
  write_override "$new_tag" "$stage_file"
  mv "$stage_file" "$active_file"
  info '镜像已就绪，正在仅替换应用容器…'
  if ! "${compose[@]}" -f "$active_file" up -d --no-deps --no-build --pull never "$service"; then
    die '应用容器重建失败；旧镜像已保留，可检查配置后用同样参数加 --rollback 回退应用镜像'
  fi
fi
info '等待服务健康检查…'
deadline=$((SECONDS + health_timeout))
healthy=0
while (( SECONDS < deadline )); do
  current=$("${compose[@]}" ps --all --quiet "$service") || current=
  if [[ -n "$current" && "$current" != *$'\n'* ]] && [[ "$(docker inspect --format '{{.State.Running}}' "$current" 2>/dev/null)" == true ]]; then
    if [[ "$(docker inspect --format '{{.Image}}' "$current" 2>/dev/null)" == "$new_image" ]] && body=$(docker exec "$current" sh -c 'wget -T 5 -q -O - "http://127.0.0.1:${CODEX_PORT:-8080}/health"' 2>/dev/null) && [[ "$body" == *'"status":"ok"'* ]]; then
      healthy=$((healthy + 1))
      if (( healthy >= 2 )); then
        info "升级成功：$body"
        if [[ -f "$previous_file" ]]; then info "旧镜像记录：${previous_file}；回退仅替换应用镜像，不恢复数据库。"; else info '未更换镜像，没有新增回退记录。'; fi
        exit 0
      fi
    else healthy=0; fi
  else healthy=0; fi
  sleep 2
done
die '健康检查未通过。请查看该应用容器日志；旧镜像仍保留，可按相同参数加 --rollback 回退应用镜像（需确认数据库版本兼容）'
