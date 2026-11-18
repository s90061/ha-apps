#!/bin/bash
# shellcheck shell=bash
# ==============================================================================
# Bridge Home Assistant /data/options.json and /config into ngosang/amule
# ==============================================================================

set -euo pipefail

OPTIONS_FILE="/data/options.json"
AMULE_CONFIG_TARGET="/config"
AMULE_HOME_LINK="/home/amule/.aMule"
STATE_FILE="${AMULE_CONFIG_TARGET}/.ha-paths-state"

json_opt() {
  local key="$1"
  local fallback="${2:-}"
  if [[ -r "${OPTIONS_FILE}" ]]; then
    jq -r --arg k "${key}" --arg d "${fallback}" 'if (.[$k] != null and (.[$k] | tostring) != "") then (.[$k] | tostring) else $d end' "${OPTIONS_FILE}"
  else
    printf '%s' "${fallback}"
  fi
}

sanitize_dir_path() {
  local raw="$1"
  local fallback="$2"
  local label="$3"
  local cleaned="${raw}"

  while [[ "${cleaned}" != "/" && "${cleaned}" == */ ]]; do
    cleaned="${cleaned%/}"
  done

  if [[ -z "${cleaned}" || "${cleaned}" == *"/../"* || "${cleaned}" == */.. || "${cleaned}" != /* ]]; then
    printf '[ha-init] Warning: invalid %s "%s", falling back to "%s"\n' "${label}" "${raw}" "${fallback}" >&2
    printf '%s' "${fallback}"
    return 0
  fi

  case "${cleaned}" in
    /share/*|/media/*|/config/*)
      printf '%s' "${cleaned}"
      ;;
    *)
      printf '[ha-init] Warning: refusing %s "%s" outside /share, /media, or /config; falling back to "%s"\n' "${label}" "${raw}" "${fallback}" >&2
      printf '%s' "${fallback}"
      ;;
  esac
}

update_amule_conf_key() {
  local file="$1"
  local key="$2"
  local val="$3"
  local tmp_file

  tmp_file="$(mktemp)"
  awk -v k="${key}" -v v="${val}" '
    BEGIN { updated = 0 }
    index($0, k "=") == 1 {
      print k "=" v
      updated = 1
      next
    }
    { print }
    END {
      if (!updated) {
        print k "=" v
      }
    }
  ' "${file}" > "${tmp_file}"
  mv "${tmp_file}" "${file}"
}

append_allowed_remote_dir() {
  local file="$1"
  local dir_path="$2"

  if [[ -f "${file}" ]] && ! grep -qxF "${dir_path}" "${file}"; then
    printf '%s\n' "${dir_path}" >> "${file}"
  fi
}

tz_opt="$(json_opt tz "Asia/Taipei")"
incoming_opt="$(sanitize_dir_path "$(json_opt incoming_dir "/share/amule/incoming")" "/share/amule/incoming" "incoming_dir")"
temp_opt="$(sanitize_dir_path "$(json_opt temp_dir "/share/amule/temp")" "/share/amule/temp" "temp_dir")"
puid_opt="$(json_opt puid "1000")"
pgid_opt="$(json_opt pgid "1000")"
umask_opt="$(json_opt umask "0002")"
fix_perm_opt="$(json_opt fix_permissions "true")"
webui_pwd_opt="$(json_opt webui_pwd "")"
webui_guest_pwd_opt="$(json_opt webui_guest_pwd "")"
gui_pwd_opt="$(json_opt gui_pwd "")"
auto_restart_opt="$(json_opt mod_auto_restart_enabled "false")"
auto_restart_cron_opt="$(json_opt mod_auto_restart_cron "0 6 * * *")"
auto_share_opt="$(json_opt mod_auto_share_enabled "false")"
auto_share_dirs_opt="$(json_opt mod_auto_share_directories "${incoming_opt}")"

if ! [[ "${puid_opt}" =~ ^[0-9]+$ ]] || [[ "${puid_opt}" -lt 1 ]] || [[ "${puid_opt}" -gt 65535 ]]; then
  printf '[ha-init] Warning: invalid puid "%s", falling back to 1000\n' "${puid_opt}" >&2
  puid_opt="1000"
fi
if ! [[ "${pgid_opt}" =~ ^[0-9]+$ ]] || [[ "${pgid_opt}" -lt 1 ]] || [[ "${pgid_opt}" -gt 65535 ]]; then
  printf '[ha-init] Warning: invalid pgid "%s", falling back to 1000\n' "${pgid_opt}" >&2
  pgid_opt="1000"
fi
if ! [[ "${umask_opt}" =~ ^[0-7]{3,4}$ ]]; then
  printf '[ha-init] Warning: invalid umask "%s", falling back to 0002\n' "${umask_opt}" >&2
  umask_opt="0002"
fi

export TZ="${tz_opt}"
export INCOMING_DIR="${incoming_opt}"
export TEMP_DIR="${temp_opt}"
export PUID="${puid_opt}"
export PGID="${pgid_opt}"
export UMASK="${umask_opt}"
export FIX_PERMISSIONS="${fix_perm_opt}"
export WEBUI_ENABLED="true"
unset WEBUI_BASE_PATH

if [[ -n "${webui_pwd_opt}" ]]; then
  export WEBUI_PWD="${webui_pwd_opt}"
else
  unset WEBUI_PWD
fi

# Defining WEBUI_GUEST_PWD as empty explicitly disables guest access in amuleapi
export WEBUI_GUEST_PWD="${webui_guest_pwd_opt}"

if [[ -n "${gui_pwd_opt}" ]]; then
  export GUI_PWD="${gui_pwd_opt}"
else
  unset GUI_PWD
fi

export MOD_AUTO_RESTART_ENABLED="${auto_restart_opt}"
export MOD_AUTO_RESTART_CRON="${auto_restart_cron_opt}"
export MOD_AUTO_SHARE_ENABLED="${auto_share_opt}"
export MOD_AUTO_SHARE_DIRECTORIES="${auto_share_dirs_opt}"

# Ensure stale auto-restart cron entry is cleared when toggled off
rm -f /etc/cron.d/amule-auto-restart

# Persist /home/amule/.aMule inside Home Assistant app_config (/config)
mkdir -p "${AMULE_CONFIG_TARGET}"
mkdir -p "$(dirname "${AMULE_HOME_LINK}")"
if [[ -L "${AMULE_HOME_LINK}" ]]; then
  ln -sfn "${AMULE_CONFIG_TARGET}" "${AMULE_HOME_LINK}"
elif [[ -d "${AMULE_HOME_LINK}" ]]; then
  cp -an "${AMULE_HOME_LINK}/." "${AMULE_CONFIG_TARGET}/" 2>/dev/null || true
  rm -rf "${AMULE_HOME_LINK}"
  ln -sfn "${AMULE_CONFIG_TARGET}" "${AMULE_HOME_LINK}"
else
  ln -sfn "${AMULE_CONFIG_TARGET}" "${AMULE_HOME_LINK}"
fi

# chown -R on a symlink does not dereference it, so fix /config ownership explicitly
chown -R "${PUID}:${PGID}" "${AMULE_CONFIG_TARGET}" 2>/dev/null || true

# Sync IncomingDir / TempDir into existing amule.conf only when HA options change
prev_incoming=""
prev_temp=""
if [[ -f "${STATE_FILE}" ]]; then
  IFS='|' read -r prev_incoming prev_temp < "${STATE_FILE}" || true
fi

amule_conf="${AMULE_CONFIG_TARGET}/amule.conf"
allowed_dirs="${AMULE_CONFIG_TARGET}/allowed-remote-dirs.dat"
if [[ -f "${amule_conf}" ]]; then
  if [[ -n "${prev_incoming}" && "${prev_incoming}" != "${INCOMING_DIR}" ]]; then
    printf '[ha-init] Updating IncomingDir in amule.conf: %s -> %s\n' "${prev_incoming}" "${INCOMING_DIR}"
    update_amule_conf_key "${amule_conf}" "IncomingDir" "${INCOMING_DIR}"
    append_allowed_remote_dir "${allowed_dirs}" "${INCOMING_DIR}"
  fi
  if [[ -n "${prev_temp}" && "${prev_temp}" != "${TEMP_DIR}" ]]; then
    printf '[ha-init] Updating TempDir in amule.conf: %s -> %s\n' "${prev_temp}" "${TEMP_DIR}"
    update_amule_conf_key "${amule_conf}" "TempDir" "${TEMP_DIR}"
    append_allowed_remote_dir "${allowed_dirs}" "${TEMP_DIR}"
  fi
fi

printf '%s|%s\n' "${INCOMING_DIR}" "${TEMP_DIR}" > "${STATE_FILE}"
chown "${PUID}:${PGID}" "${STATE_FILE}" 2>/dev/null || true

printf '[ha-init] Launching ngosang/amule entrypoint (v%s, PUID=%s, PGID=%s, INCOMING_DIR=%s, TEMP_DIR=%s)\n' \
  "${APP_VERSION:-dev}" "${PUID}" "${PGID}" "${INCOMING_DIR}" "${TEMP_DIR}"

exec /home/amule/entrypoint.sh
