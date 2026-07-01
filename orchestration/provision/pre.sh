#!/usr/bin/env bash
# Runs as the steam user before the CS2 server launches.

( set -euo pipefail

GAMEINFO="/home/steam/cs2-dedicated/game/csgo/gameinfo.gi"
log() { echo "[hook] $*"; }

if ! grep -q "csgo/addons/metamod" "${GAMEINFO}"; then
  log "patching gameinfo.gi search paths"
  tmp="$(mktemp)"
  awk '
    { print }
    /Game_LowViolence/ && !done { print "\t\t\tGame\tcsgo/addons/metamod"; done=1 }
  ' "${GAMEINFO}" > "${tmp}" && mv "${tmp}" "${GAMEINFO}"
  grep -q "csgo/addons/metamod" "${GAMEINFO}" \
    || log "WARNING: gameinfo.gi anchor not found; MetaMod will NOT load"
fi

log "boot hook complete"
) || echo "[hook] pre.sh hit an error; continuing so the CS2 server still launches"
