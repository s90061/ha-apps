#!/bin/bash
# shellcheck shell=bash
# ==============================================================================
# Bridge Home Assistant /data/options.json into GridCraft Server
# ==============================================================================

set -euo pipefail

OPTIONS_FILE="/data/options.json"

json_opt() {
  local key="$1"
  local fallback="${2:-}"
  if [[ -r "${OPTIONS_FILE}" ]]; then
    jq -r --arg k "${key}" --arg d "${fallback}" 'if (.[$k] != null and (.[$k] | tostring) != "") then (.[$k] | tostring) else $d end' "${OPTIONS_FILE}"
  else
    printf '%s' "${fallback}"
  fi
}

sanitize_storage_path() {
  local raw="$1"
  local fallback="$2"
  local cleaned="${raw}"

  while [[ "${cleaned}" != "/" && "${cleaned}" == */ ]]; do
    cleaned="${cleaned%/}"
  done

  if [[ -z "${cleaned}" || "${cleaned}" == *"/../"* || "${cleaned}" == */.. || "${cleaned}" != /* ]]; then
    printf '[ha-init] Warning: invalid storage_dir "%s", falling back to "%s"\n' "${raw}" "${fallback}" >&2
    printf '%s' "${fallback}"
    return 0
  fi

  case "${cleaned}" in
    /share|/share/*|/config|/config/*|/media|/media/*)
      printf '%s' "${cleaned}"
      ;;
    *)
      printf '[ha-init] Warning: refusing storage_dir "%s" outside /share, /config, or /media; falling back to "%s"\n' "${raw}" "${fallback}" >&2
      printf '%s' "${fallback}"
      ;;
  esac
}

tz_opt="$(json_opt tz "Asia/Taipei")"
storage_dir_opt="$(sanitize_storage_path "$(json_opt storage_dir "/share/gridcraft")" "/share/gridcraft")"
default_lang_opt="$(json_opt default_language "zh-hant")"
author_name_opt="$(json_opt author_name "Home Assistant")"
also_dl_opt="$(json_opt also_download_on_save "false")"

mkdir -p "${storage_dir_opt}"

export TZ="${tz_opt}"
export PORT="8090"
export WEB_DIR="/app/web"
export STORAGE_DIR="${storage_dir_opt}"
export DEFAULT_LANGUAGE="${default_lang_opt}"
export AUTHOR_NAME="${author_name_opt}"
export ALSO_DOWNLOAD_ON_SAVE="${also_dl_opt}"

printf '[ha-init] Starting GridCraft v%s (STORAGE_DIR=%s, DEFAULT_LANGUAGE=%s, AUTHOR_NAME=%s)\n' \
  "${GRIDCRAFT_VERSION:-0.19.1}" "${STORAGE_DIR}" "${DEFAULT_LANGUAGE}" "${AUTHOR_NAME}"

exec python3 /app/server.py
