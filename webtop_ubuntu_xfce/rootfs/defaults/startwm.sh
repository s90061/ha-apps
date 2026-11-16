#!/bin/bash
# shellcheck shell=bash

# Default settings
if [ ! -d "${HOME}/.config/xfce4/xfconf/xfce-perchannel-xml" ]; then
  mkdir -p "${HOME}/.config/xfce4/xfconf/xfce-perchannel-xml"
  cp /defaults/xfce/* "${HOME}/.config/xfce4/xfconf/xfce-perchannel-xml/"
fi

mkdir -p "${HOME}/.local/state"

# Start DE and retain session logs under /config/.local/state for diagnostics
exec dbus-launch --exit-with-session /usr/bin/xfce4-session >"${HOME}/.local/state/xfce4-session.log" 2>&1
