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
  if [[ "${#admin_pass_opt}" -lt 4 ]] || [[ "${#admin_pass_opt}" -gt 128 ]]; then
    printf '[ha-init] Warning: admin_password must be 4-128 characters; ignoring override\n' >&2
    admin_pass_opt=""
  fi
fi

if [[ -n "${admin_pass_opt}" ]]; then
  export ADMIN_PASSWORD="${admin_pass_opt}"
  export FORCE_ADMIN_PASSWORD="true"
  printf '[ha-init] Syncing PiStream admin account "%s" password from Home Assistant configuration\n' "${ADMIN_USER}"
else
  unset FORCE_ADMIN_PASSWORD
  if [[ ! -s "${DB_FILE}" ]]; then
    gen_pass="$(LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 16 || true)"
    if [[ "${#gen_pass}" -lt 12 ]]; then
      gen_pass="$(od -An -N12 -tx1 /dev/urandom | tr -d ' \n')"
    fi
    export ADMIN_PASSWORD="${gen_pass}"
    printf '[ha-init] ==================================================\n'
    printf '[ha-init] Initial PiStream Admin user: %s\n' "${ADMIN_USER}"
    printf '[ha-init] Initial PiStream Admin password: %s\n' "${ADMIN_PASSWORD}"
    printf '[ha-init] ==================================================\n'
  else
    export ADMIN_PASSWORD="keep_existing_password"
    chmod 600 "${DB_FILE}" 2>/dev/null || true
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

probe_dev_access() {
  local pattern="$1"
  local out=""
  local dev name status
  shopt -s nullglob
  # shellcheck disable=SC2206
  local matches=( ${pattern} )
  shopt -u nullglob
  for dev in "${matches[@]}"; do
    [[ -e "${dev}" ]] || continue
    name="$(basename "${dev}")"
    if ( : <> "${dev}" ) 2>/dev/null; then
      status="rw"
    else
      status="blocked"
    fi
    if [[ -n "${out}" ]]; then
      out="${out},${name}(${status})"
    else
      out="${name}(${status})"
    fi
  done
  printf '%s' "${out:-none}"
}

if [[ "${hw_hevc_opt}" == "true" ]]; then
  if [[ -d /dev/dri ]] || compgen -G "/dev/video*" >/dev/null 2>&1; then
    export HW_ACCEL="drm"
  else
    unset HW_ACCEL
    printf '[ha-init] hw_hevc_decode enabled but no /dev/dri or /dev/video* device found; falling back to CPU decode\n'
  fi
  udev_status="no"
  if [[ -d /run/udev ]]; then
    udev_status="yes"
  fi
  media_status="$(probe_dev_access '/dev/media*')"
  dma_status="$(probe_dev_access '/dev/dma_heap/* /dev/udmabuf')"
  video19_status="$(probe_dev_access '/dev/video19')"
  printf '[ha-init] HW HEVC probe: udev=%s media=%s video19=%s dma_heap=%s\n' \
    "${udev_status}" "${media_status}" "${video19_status}" "${dma_status}"
else
  unset HW_ACCEL
fi

printf '[ha-init] Starting PiStream v%s (MEDIA_DIR=%s, TRANSCODE_TEMP_DIR=%s, SEGMENT_DURATION=%ss, MAX_KEEP_SEGMENTS=%s, HW_ACCEL=%s)\n' \
  "${APP_VERSION:-dev}" "${MEDIA_DIR}" "${TRANSCODE_TEMP_DIR}" "${SEGMENT_DURATION}" "${MAX_KEEP_SEGMENTS}" "${HW_ACCEL:-none}"

exec /app/pistream
