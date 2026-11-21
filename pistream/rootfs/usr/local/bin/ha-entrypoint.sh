#!/bin/bash
# shellcheck shell=bash
# ==============================================================================
# Bridge Home Assistant /data/options.json and /config into PiStream
# ==============================================================================

set -euo pipefail

OPTIONS_FILE="/data/options.json"
CONFIG_DIR="/config"
DB_FILE="${CONFIG_DIR}/pistream.db"
JWT_SECRET_FILE="${CONFIG_DIR}/.jwt_secret"

json_opt() {
  local key="$1"
  local fallback="${2:-}"
  if [[ -r "${OPTIONS_FILE}" ]]; then
    jq -r --arg k "${key}" --arg d "${fallback}" 'if (.[$k] != null and (.[$k] | tostring) != "") then (.[$k] | tostring) else $d end' "${OPTIONS_FILE}"
  else
    printf '%s' "${fallback}"
  fi
}

sanitize_media_path() {
  local raw="$1"
  local fallback="$2"
  local cleaned="${raw}"

  while [[ "${cleaned}" != "/" && "${cleaned}" == */ ]]; do
    cleaned="${cleaned%/}"
  done

  if [[ -z "${cleaned}" || "${cleaned}" == *"/../"* || "${cleaned}" == */.. || "${cleaned}" != /* ]]; then
    printf '[ha-init] Warning: invalid media_dir "%s", falling back to "%s"\n' "${raw}" "${fallback}" >&2
    printf '%s' "${fallback}"
    return 0
  fi

  case "${cleaned}" in
    /media|/media/*|/share|/share/*|/config|/config/*)
      printf '%s' "${cleaned}"
      ;;
    *)
      printf '[ha-init] Warning: refusing media_dir "%s" outside /media, /share, or /config; falling back to "%s"\n' "${raw}" "${fallback}" >&2
      printf '%s' "${fallback}"
      ;;
  esac
}

tz_opt="$(json_opt tz "Asia/Taipei")"
media_dir_opt="$(sanitize_media_path "$(json_opt media_dir "/media")" "/media")"
admin_user_opt="$(json_opt admin_user "admin")"
admin_pass_opt="$(json_opt admin_password "")"
jwt_secret_opt="$(json_opt jwt_secret "")"
seg_dur_opt="$(json_opt segment_duration "4")"
max_keep_opt="$(json_opt max_keep_segments "10")"
storage_opt="$(json_opt transcode_storage "auto")"
hw_hevc_opt="$(json_opt hw_hevc_decode "true")"

if [[ -z "${admin_user_opt// /}" ]]; then
  admin_user_opt="admin"
fi

if ! [[ "${seg_dur_opt}" =~ ^[0-9]+$ ]] || [[ "${seg_dur_opt}" -lt 2 ]] || [[ "${seg_dur_opt}" -gt 10 ]]; then
  seg_dur_opt="4"
fi

if ! [[ "${max_keep_opt}" =~ ^[0-9]+$ ]] || [[ "${max_keep_opt}" -lt 4 ]] || [[ "${max_keep_opt}" -gt 60 ]]; then
  max_keep_opt="10"
fi

mkdir -p "${CONFIG_DIR}/cache/thumbs" "${CONFIG_DIR}/cache/subs"

# Persist JWT secret across container restarts unless overridden in options
if [[ -n "${jwt_secret_opt}" ]]; then
  export JWT_SECRET="${jwt_secret_opt}"
elif [[ -s "${JWT_SECRET_FILE}" ]]; then
  JWT_SECRET="$(tr -d '\r\n' < "${JWT_SECRET_FILE}")"
  export JWT_SECRET
else
  JWT_SECRET="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
  printf '%s\n' "${JWT_SECRET}" > "${JWT_SECRET_FILE}"
  chmod 600 "${JWT_SECRET_FILE}"
  export JWT_SECRET
fi

# Handle initial or forced admin password
export ADMIN_USER="${admin_user_opt}"
if [[ -n "${admin_pass_opt}" ]]; then
  if [[ "${#admin_pass_opt}" -lt 4 ]]; then
    printf '[ha-init] Warning: admin_password must be at least 4 characters; ignoring override\n' >&2
    unset FORCE_ADMIN_PASSWORD
    export ADMIN_PASSWORD="admin123"
  else
    export ADMIN_PASSWORD="${admin_pass_opt}"
    export FORCE_ADMIN_PASSWORD="true"
    printf '[ha-init] Syncing PiStream admin account "%s" password from Home Assistant configuration\n' "${ADMIN_USER}"
  fi
else
  unset FORCE_ADMIN_PASSWORD
  if [[ ! -s "${DB_FILE}" ]]; then
    gen_pass="$(LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 14 || true)"
    if [[ "${#gen_pass}" -lt 8 ]]; then
      gen_pass="admin123"
    fi
    export ADMIN_PASSWORD="${gen_pass}"
    printf '[ha-init] ==================================================\n'
    printf '[ha-init] Initial PiStream Admin user: %s\n' "${ADMIN_USER}"
    printf '[ha-init] Initial PiStream Admin password: %s\n' "${ADMIN_PASSWORD}"
    printf '[ha-init] ==================================================\n'
  else
    export ADMIN_PASSWORD="keep_existing_password"
  fi
fi

# Choose HLS transcode buffer location (auto detects if /dev/shm >= 256MB)
transcode_dir="/tmp/pistream_hls"
case "${storage_opt}" in
  shm)
    transcode_dir="/dev/shm/pistream_hls"
    ;;
  tmp)
    transcode_dir="/tmp/pistream_hls"
    ;;
  *)
    shm_kb="$(df -Pk /dev/shm 2>/dev/null | awk 'NR==2 {print $2}' || true)"
    if [[ "${shm_kb:-0}" =~ ^[0-9]+$ ]] && [[ "${shm_kb}" -ge 262144 ]]; then
      transcode_dir="/dev/shm/pistream_hls"
    else
      shm_mb=$(( ${shm_kb:-0} / 1024 ))
      printf '[ha-init] /dev/shm size is %sMB (<256MB); using /tmp/pistream_hls to avoid 4K Remux OOM\n' "${shm_mb}"
      transcode_dir="/tmp/pistream_hls"
    fi
    ;;
esac

rm -rf "${transcode_dir}"
mkdir -p "${transcode_dir}"

export TZ="${tz_opt}"
export PORT="8080"
export DB_PATH="${DB_FILE}"
export MEDIA_DIR="${media_dir_opt}"
export CACHE_DIR="${CONFIG_DIR}/cache"
export TRANSCODE_TEMP_DIR="${transcode_dir}"
export WEB_DIR="/app/web/dist"
export SEGMENT_DURATION="${seg_dur_opt}"
export MAX_KEEP_SEGMENTS="${max_keep_opt}"

if [[ "${hw_hevc_opt}" == "true" ]]; then
  if [[ -d /dev/dri ]] || compgen -G "/dev/video*" >/dev/null 2>&1; then
    export HW_ACCEL="drm"
  else
    unset HW_ACCEL
    printf '[ha-init] hw_hevc_decode enabled but no /dev/dri or /dev/video* device found; falling back to CPU decode\n'
  fi
else
  unset HW_ACCEL
fi

printf '[ha-init] Starting PiStream v%s (MEDIA_DIR=%s, TRANSCODE_TEMP_DIR=%s, SEGMENT_DURATION=%ss, MAX_KEEP_SEGMENTS=%s, HW_ACCEL=%s)\n' \
  "${APP_VERSION:-dev}" "${MEDIA_DIR}" "${TRANSCODE_TEMP_DIR}" "${SEGMENT_DURATION}" "${MAX_KEEP_SEGMENTS}" "${HW_ACCEL:-none}"

exec /app/pistream
