#!/usr/bin/env bash
# Runs (as the `steam` user) before the CS2 server launches, on every boot.
#
# The orchestrator bakes the shared base (game + MetaMod + CounterStrikeSharp)
# and, per instance, writes the plugins and their templated DB configs into the
# overlay before launch. The only per-boot work left here is registering MetaMod
# in gameinfo.gi — a game file SteamCMD can rewrite on update, so we (re)patch it
# every boot. Idempotent: insert `Game csgo/addons/metamod` right after the
# Game_LowViolence anchor line.

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
    || log "WARNING: gameinfo.gi anchor not found — MetaMod will NOT load (CS2 may have changed the file format)"
fi

log "boot hook complete"
) || echo "[hook] pre.sh hit an error — continuing so the CS2 server still launches"
