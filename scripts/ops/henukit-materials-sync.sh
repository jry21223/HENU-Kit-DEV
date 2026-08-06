#!/usr/bin/env bash
# 资料库 webhook 驱动:一次推送触发 镜像文件 -> 转 Slides -> 归一化导入。
#
# 被 deploy-webhook runner 以固定参数调用(--sha/--delivery/--repository/--ref,
# 见 services/deploy-webhook/internal/runner)。本脚本只关心内容本身:无论推送
# 哪个 SHA,镜像总是同步仓库当前默认分支并重建。全部步骤幂等,可安全重跑。
#
# 环境变量:
#   HENUKIT_MATERIALS_ROOT          镜像根(默认 /opt/henukit-materials)
#   HENUKIT_MATERIALS_REPO_URL      源仓库(默认 https://github.com/jry21223/HENU-Final-Review.git)
#   HENUKIT_MATERIALS_REPO_REF      源分支(默认 main)
#   HENUKIT_MATERIALS_DATABASE_URL  直接数据库连接串(设置后使用主机 psql,忽略 compose)
#   HENUKIT_MATERIALS_RELEASE_DIR   release 目录(含 docker-compose.henukit.release.yml,
#                                   默认取 /opt/henukit-releases 下最新的一个)
#   HENUKIT_MATERIALS_ENV_FILE      生产 Compose 环境文件(默认 /opt/henukit/.env.henukit)
#
# 依赖: git python3(python-pptx) node docker;.ppt 转换还需要 soffice。
set -Eeuo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
root="${HENUKIT_MATERIALS_ROOT:-/opt/henukit-materials}"
export HENUKIT_MATERIALS_ROOT="$root"

die() {
  echo "henukit-materials-sync: $*" >&2
  exit 1
}

command -v node >/dev/null || die "node is required"
command -v python3 >/dev/null || die "python3 is required"

# 1) 文件镜像:只发布 manifest 中非"待复核"资产,原子换入。
"$script_dir/sync-henukit-materials.sh"

manifest="$root/repo/manifest.json"
[[ -f "$manifest" ]] || die "manifest.json missing after sync: $manifest"

# 2) 幻灯片转换:PPT 课件转门户 Slides JSON。转换失败不阻塞文件同步与导入,
#    依赖缺失(退出码 2/3)或单文件失败(退出码 5)时仅告警。
slides_dir="$root/slides"
if python3 "$script_dir/convert-henukit-slides.py" \
  --mirror "$root/public" --out "$slides_dir" --manifest "$manifest"; then
  echo "henukit-materials-sync: slides conversion ok"
else
  echo "henukit-materials-sync: WARNING slides conversion failed (files still synced)" >&2
fi

# 3) 贡献者归属:这些是别人的笔记、真题和课件,署名应归实际贡献者而不是平台。
#    manifest 不记作者,镜像又是浅克隆没有历史,所以从 GitHub API 取提交历史。
#    取不到时资料不署名,不阻塞同步。
contributors_file="$root/contributors.json"
if node "$script_dir/fetch-henukit-contributors.mjs" --out "$contributors_file"; then
  echo "henukit-materials-sync: contributor attribution ok"
else
  echo "henukit-materials-sync: WARNING contributor attribution failed (materials stay uncredited)" >&2
  contributors_file=""
fi

# 4) 归一化导入 study 库(courses/materials)。
import_sql() {
  node "$script_dir/import-henukit-materials.mjs" \
    --manifest "$manifest" --slides-dir "$slides_dir" \
    ${contributors_file:+--contributors "$contributors_file"}
}

if [[ -n "${HENUKIT_MATERIALS_DATABASE_URL:-}" ]]; then
  command -v psql >/dev/null || die "HENUKIT_MATERIALS_DATABASE_URL set but psql is missing"
  echo "henukit-materials-sync: importing via direct database URL"
  import_sql | psql "$HENUKIT_MATERIALS_DATABASE_URL" -v ON_ERROR_STOP=1 -f -
  exit 0
fi

compose_dir="${HENUKIT_MATERIALS_RELEASE_DIR:-}"
if [[ -z "$compose_dir" ]]; then
  compose_dir="$(ls -dt /opt/henukit-releases/*/ 2>/dev/null | head -1)"
fi
compose_dir="${compose_dir%/}"
compose_file="$compose_dir/docker-compose.henukit.release.yml"
env_file="${HENUKIT_MATERIALS_ENV_FILE:-/opt/henukit/.env.henukit}"
[[ -f "$compose_file" ]] || die "release compose not found: $compose_file"
[[ -f "$env_file" ]] || die "production env file not found: $env_file"

compose=(docker compose --env-file "$env_file" -f "$compose_file")

# study 库名:优先取 env 文件里的 STUDY_DATABASE_URL,否则按 compose 默认。
db_url="$(grep -E '^STUDY_DATABASE_URL=' "$env_file" | tail -1 | cut -d= -f2- || true)"
if [[ -z "$db_url" ]]; then
  db_url="postgres://henukit:henukit_dev_change_me@postgres:5432/study?sslmode=disable"
fi
dbname="$(printf '%s' "$db_url" | sed -E 's|^[^/]+//[^/]+/([^?]+).*|\1|')"
[[ "$dbname" =~ ^[A-Za-z0-9_-]+$ ]] || die "cannot extract database name from: $db_url"
postgres_user="${POSTGRES_USER:-}"
if [[ -z "$postgres_user" ]]; then
  postgres_user="$(grep -E '^POSTGRES_USER=' "$env_file" | tail -1 | cut -d= -f2- || true)"
fi
[[ -n "$postgres_user" ]] || die "POSTGRES_USER not found in $env_file"

psql_in() {
  "${compose[@]}" exec -T postgres psql -U "$postgres_user" -v ON_ERROR_STOP=1 "$@"
}

if ! psql_in -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='$dbname'" | grep -q 1; then
  echo "henukit-materials-sync: creating study database $dbname"
  psql_in -d postgres -c "CREATE DATABASE \"$dbname\""
fi

echo "henukit-materials-sync: importing into database $dbname"
import_sql | psql_in -d "$dbname" -f -
