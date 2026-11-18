# Home Assistant App: aMule

## Overview

This app packages [`ngosang/amule`](https://github.com/ngosang/docker-amule) (`3.2.0-1`, pinned multi-arch OCI digest `sha256:4b1a4246d57e7994b31c723b8c55474654e328c1b8eb0f9deb7f1843673bd515` for `aarch64`) for Home Assistant Supervisor, providing `amuled` alongside the built-in `amuleapi` Web UI and REST API.

## Accessing the Web UI

1. **Home Assistant Ingress (Sidebar / Open Web UI)**: Enabled by default. An internal Nginx proxy rewrites `amuleapi` session cookie paths dynamically to match `/api/hassio_ingress/<token>/api/v1` and keeps SSE live updates unbuffered.
2. **First-time Login Password**:
   - Set `webui_pwd` in the app **Configuration** tab before starting, **or**
   - Leave `webui_pwd` blank on first start: `ngosang/amule` will generate a random 14-character password and print `[INIT] Web UI password: <password>` in the app **Log** tab.
   - Note: While `webui_pwd` is set in Configuration, it overwrites the admin password on every restart. Leaving it blank after initialization preserves the existing `amuleapi-passwords` file.
3. **Optional Direct Ports (`4711/tcp`, `4712/tcp`)**: Both are disabled (`null`) by default so the Web UI is exposed only through authenticated Home Assistant Ingress. Map `4711/tcp` (REST API / direct HTTP Web UI behind TLS reverse proxy) or `4712/tcp` (`amulegui` / `amulecmd`) in the **Network** section only if needed.

## Router Port Forwarding (High ID)

To obtain an ED2K **High ID** and optimal Kademlia connectivity, forward these ports on your router to your Home Assistant host:

* `4662/tcp` — ED2K client-to-client TCP
* `4665/udp` — ED2K server UDP (global search; `4662 + 3`)
* `4672/udp` — Extended eMule protocol & Kademlia UDP

## Storage Persistence

* **`/config` (`app_config:rw`)**: Linked to `/home/amule/.aMule` inside the container and persisted under `/addon_configs/<repo>_amule` on the host (`amule.conf`, `amuleapi.conf`, `nodes.dat`, `known.met`, `allowed-remote-dirs.dat`).
* **`/share` (`share:rw`) & `/media` (`media:rw`)**: Default downloads go to `/share/amule/incoming` and `/share/amule/temp`. Keeping both on the same mount allows instant moves on download completion.

> [!NOTE]
> Always stop the app before editing `amule.conf` manually on disk, because `amuled` keeps configuration in memory and rewrites `amule.conf` on shutdown.

## Configuration Options

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `tz` | `str` | `Asia/Taipei` | Container timezone (`TZ`). |
| `incoming_dir` | `match(^/(share\|media\|config)/.+$)` | `/share/amule/incoming` | Completed downloads folder (`INCOMING_DIR`). |
| `temp_dir` | `match(^/(share\|media\|config)/.+$)` | `/share/amule/temp` | Incomplete downloads folder (`TEMP_DIR`). |
| `puid` | `int(1,65535)` | `1000` | UID for `amule` daemon and file ownership (`PUID`). |
| `pgid` | `int(1,65535)` | `1000` | GID for `amule` daemon and file ownership (`PGID`). |
| `umask` | `match(^[0-7]{3,4}$)` | `0002` | File creation umask (`UMASK`). |
| `fix_permissions` | `bool` | `true` | Chown `incoming_dir` and `temp_dir` to `PUID:PGID` on boot. |
| `webui_pwd` | `password?` | _(unset)_ | Web UI / REST API admin password (`WEBUI_PWD`). |
| `webui_guest_pwd` | `password?` | _(unset)_ | Optional read-only Web UI guest password (`WEBUI_GUEST_PWD`); blank disables guest access. |
| `gui_pwd` | `password?` | _(unset)_ | Optional External Connections password (`GUI_PWD`) for `4712/tcp`. |
| `mod_auto_restart_enabled` | `bool` | `false` | Enable scheduled `amuled` restart via cron. |
| `mod_auto_restart_cron` | `str` | `0 6 * * *` | Cron schedule used when `mod_auto_restart_enabled` is true. |
| `mod_auto_share_enabled` | `bool` | `false` | Rewrite recursive shared roots (`shareddir-recursive.dat`) on startup. |
| `mod_auto_share_directories` | `str` | `/share/amule/incoming` | Semicolon-separated directories shared recursively when `mod_auto_share_enabled` is true. |
