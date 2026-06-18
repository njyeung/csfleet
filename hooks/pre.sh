#!/usr/bin/env bash
# Runs (as the `steam` user) before the CS2 server launches, on every boot.
#
# The shared base install (game + MetaMod + CounterStrikeSharp + InspectGive +
# WeaponPaints + core.json) is baked by the orchestrator and presented to the
# container through the overlay, so there's nothing to copy in here. This hook
# only does the per-boot / per-instance work that can't live in the shared,
# read-only base:
#
#   1. patch gameinfo.gi so MetaMod loads
#   2. template the DB-backed plugin configs from the container's env
#   3. install the per-server cvar config (gamemode_competitive_server.cfg)

( set -euo pipefail

GAME_DIR="/home/steam/cs2-dedicated/game/csgo"
CSS_DIR="${GAME_DIR}/addons/counterstrikesharp"
GAMEINFO="${GAME_DIR}/gameinfo.gi"
CFG_DIR="${GAME_DIR}/cfg"

# Per-server cvar config. The orchestrator points CS2_GAMEMODE_CFG at a file to
# install; if it's unset/missing we install no cvar config at all.
CS2_GAMEMODE_CFG="${CS2_GAMEMODE_CFG:-}"

log() { echo "[hook] $*"; }

# ── 1. Register MetaMod in gameinfo.gi's SearchPaths ─────────────────────────
# The mods are baked into the shared base, but gameinfo.gi is a game file that
# SteamCMD can rewrite on update, so we (re)patch it per boot. Idempotent: insert
# `Game csgo/addons/metamod` right after the Game_LowViolence anchor line.

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

# ── 2. Template the DB-backed plugin configs ─────────────────────────────────
# WeaponPaints.json and InspectGive.json point at the same MariaDB, templated
# from the WP_DB_* env vars the container is started with — per-deployment, so
# they can't be baked into the shared base.

log "writing WeaponPaints.json"
WP_CFG_DIR="${CSS_DIR}/configs/plugins/WeaponPaints"
mkdir -p "${WP_CFG_DIR}"
cat > "${WP_CFG_DIR}/WeaponPaints.json" <<JSON
{
  "DatabaseHost": "${WP_DB_HOST}",
  "DatabasePort": ${WP_DB_PORT},
  "DatabaseName": "${WP_DB_NAME}",
  "DatabaseUser": "${WP_DB_USER}",
  "DatabasePassword": "${WP_DB_PASS}",
  "Additional": {
    "CommandKnife": [ "knife", "knifes", "knives" ],
    "CommandSkin": [ "ws", "skin", "skins" ],
    "CommandRefresh": [ "wp" ],
    "CommandWpEnabled": true,
    "CommandWearUpdate": true,
    "GiveRandomSkin": false,
    "SkinEnabled": true,
    "KnifeEnabled": true,
    "GloveEnabled": true,
    "AgentEnabled": true,
    "MusicEnabled": true,
    "StickerEnabled": true,
    "KeychainEnabled": true
  }
}
JSON

log "writing InspectGive.json"
IG_CFG_DIR="${CSS_DIR}/configs/plugins/InspectGive"
mkdir -p "${IG_CFG_DIR}"
cat > "${IG_CFG_DIR}/InspectGive.json" <<JSON
{
  "DatabaseHost": "${WP_DB_HOST}",
  "DatabasePort": ${WP_DB_PORT},
  "DatabaseName": "${WP_DB_NAME}",
  "DatabaseUser": "${WP_DB_USER}",
  "DatabasePassword": "${WP_DB_PASS}",
  "ApplyToBothTeams": true,
  "ChatTriggerPrefix": "csgo_econ_action_preview",
  "ConfigVersion": 1
}
JSON

# ── 3. Install the per-server cvar config ────────────────────────────────────
# We run the default game mode (competitive), so CS2 execs gamemode_competitive.cfg
# then its user-override gamemode_competitive_server.cfg. We own the latter.
#
# The orchestrator hands us a config via CS2_GAMEMODE_CFG. If it's unset we install
# no config at all — and clear any cfg a previous boot left behind, so "no config"
# stays true on restart.

mkdir -p "${CFG_DIR}"
DEST="${CFG_DIR}/gamemode_competitive_server.cfg"
if [[ -n "${CS2_GAMEMODE_CFG}" && -f "${CS2_GAMEMODE_CFG}" ]]; then
  log "installing injected cvar config from ${CS2_GAMEMODE_CFG}"
  cp -f "${CS2_GAMEMODE_CFG}" "${DEST}"
else
  [[ -n "${CS2_GAMEMODE_CFG}" ]] \
    && log "WARNING: CS2_GAMEMODE_CFG=${CS2_GAMEMODE_CFG} not found — installing no cvar config"
  log "no cvar config provided — leaving gamemode_competitive_server.cfg unset"
  rm -f "${DEST}"
fi

log "boot hook complete"
) || echo "[hook] pre.sh hit an error — continuing so the CS2 server still launches"
