# Home Assistant App: Hello World

## Overview

This minimal reference app runs an `s6-overlay` v3 service on Home Assistant (`amd64` / `aarch64`) that periodically logs the running app version and your configured message. It does not open any network ports and has zero conflicts with other apps.

## Configuration

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `message` | `str` | `Hello from S90061 Home Assistant Apps!` | Greeting text printed to the app log on each interval. |
| `interval_seconds` | `int(5,86400)` | `60` | Delay in seconds between log entries (minimum `5`, maximum `86400`). |

## Verifying Operation & Updates

1. Start the app and open the **Log** tab in Home Assistant.
2. Confirm lines like `[v0.1.0] Hello from S90061 Home Assistant Apps!` appear cleanly with no interpreter or permission errors.
3. When a newer version is published to the repository's `main` branch, Home Assistant Supervisor will surface an update notification in **Settings > Updates** and on the app page.
