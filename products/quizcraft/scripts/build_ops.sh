#!/bin/bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
STATIC_DEPLOY_DIR="${STATIC_DEPLOY_DIR:-}"

_validate_static_deploy_dir() {
  local deploy_dir="$1"
  local resolved_dir

  if [ -d "${deploy_dir}" ]; then
    resolved_dir="$(cd "${deploy_dir}" && pwd -P)"
  elif [ "${deploy_dir#/}" != "${deploy_dir}" ]; then
    resolved_dir="${deploy_dir%/}"
  else
    resolved_dir="$(cd "$(dirname "${deploy_dir}")" 2>/dev/null && pwd -P)/${deploy_dir##*/}"
  fi
  case "${resolved_dir}/" in
    /var/www/quizcraft-cn/*|/opt/quizcraft-cn/static/*)
      ;;
    *)
      echo "refusing to deploy outside approved static roots: ${resolved_dir}" >&2
      echo "set STATIC_DEPLOY_DIR under /var/www/quizcraft-cn or /opt/quizcraft-cn/static" >&2
      exit 1
      ;;
  esac

  if [ ! -d "${resolved_dir}" ]; then
    echo "static deploy dir does not exist: ${resolved_dir}" >&2
    exit 1
  fi

  printf '%s\n' "${resolved_dir}"
}

cd "${ROOT_DIR}/web-app"
npm run build:ops

if [ -n "${STATIC_DEPLOY_DIR}" ]; then
  STATIC_DEPLOY_DIR="$(_validate_static_deploy_dir "${STATIC_DEPLOY_DIR}")"
  cp -f dist/index.html "${STATIC_DEPLOY_DIR}/"
  rm -rf "${STATIC_DEPLOY_DIR}/assets"
  mkdir -p "${STATIC_DEPLOY_DIR}/assets"
  cp -rf dist/assets/. "${STATIC_DEPLOY_DIR}/assets/"
  if [ -f dist/wechat-receive-qrcode.jpg ]; then
    cp -f dist/wechat-receive-qrcode.jpg "${STATIC_DEPLOY_DIR}/"
  fi
  if [ -f dist/henu-kit-qq-group.png ]; then
    cp -f dist/henu-kit-qq-group.png "${STATIC_DEPLOY_DIR}/"
  fi
  chmod 644 "${STATIC_DEPLOY_DIR}/wechat-receive-qrcode.jpg" 2>/dev/null || true
  chmod 644 "${STATIC_DEPLOY_DIR}/henu-kit-qq-group.png" 2>/dev/null || true
fi
