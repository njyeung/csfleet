#!/usr/bin/env python3
"""
End-to-end smoke test for the orchestrator control-plane API.

Scenario (mirrors the scripted steps):
  1. Create a standalone server on a port (accepting, no restart/stop cadence).
     Watch it come up over SSE.
  2. List servers.
  3. Upload skininspect.cfg + the WeaponPaints plugin and its dependencies, then
     create a cluster that assigns them.
  4. Add a member that inherits the cluster's plugins/configs.
  5. Add a member that explicitly overrides with NO plugins / NO configs.
  6. Stop the standalone server.
  7. Stop the cluster (its members).

Start the orchestrator first (fresh DB, API on :8080), then:  python3 scripts/test.py
Standard library only — no pip installs.
"""

import json
import threading
import time
import urllib.error
import urllib.request

BASE = "http://localhost:8080"

# host port assignments (must not collide across servers/clusters)
STANDALONE = "inspect-1"
STANDALONE_PORT = 27015
CLUSTER = "comp"
CLUSTER_PORT = 27016
MEMBER_INHERIT = "comp-a"
MEMBER_NONE = "comp-b"

# WeaponPaints + the three plugins it `requires`. We upload each manifest under
# its catalog key (the name in the URL; manifests no longer carry their own name)
# and assign all four to the cluster.
WEAPONPAINTS_DEPS = ["AnyBaseLib", "PlayerSettings", "MenuManager"]
CLUSTER_PLUGINS = ["WeaponPaints"] + WEAPONPAINTS_DEPS
# Catalog name (the PK / identifier) and the on-disk filename the body lands at.
# The filename is resolved under game/csgo/cfg/, so it's just a bare name.
CLUSTER_CONFIG = "skininspect.cfg"
CLUSTER_CONFIG_FILENAME = "gamemode_competitive_server.cfg"

# ---------------------------------------------------------------------------
# Inlined artifacts (copied from config/ and plugins/examples/).
# ---------------------------------------------------------------------------

SKININSPECT_CFG = """\
// Skin-inspect cvars. Installed as gamemode_competitive_server.cfg by hooks/pre.sh
// when CS2_GAMEMODE_CFG points here. Round + match never end, no warmup, you can't
// get stranded dead, bunny hop + buy-anywhere QoL. See README for the rationale.

mp_ignore_round_win_conditions 1
mp_roundtime 0
mp_roundtime_defuse 0
mp_freezetime 0
mp_buy_anywhere 1
mp_buytime 9999
mp_warmup_end
mp_warmuptime 0

bot_quota 0
mp_autokick 0
mp_autoteambalance 0
mp_limitteams 0
mp_force_pick_time 0

sv_cheats 1
sv_falldamage_scale 0

// No player-vs-player damage
mp_damage_scale_ct_body 0
mp_damage_scale_ct_head 0
mp_damage_scale_t_body 0
mp_damage_scale_t_head 0
mp_respawn_on_death_ct 1
mp_respawn_on_death_t 1

// Auto bunny hop
sv_autobunnyhopping 1
sv_enablebunnyhopping 1

// Inspect QoL
sv_infinite_ammo 1
mp_buy_anywhere 1
mp_buytime 60000
mp_maxmoney 60000
mp_startmoney 60000
mp_free_armor 2
"""

MANIFEST_WEAPONPAINTS = """\
# WeaponPaints — skins/knives/gloves/agents, backed by MariaDB.
requires = ["AnyBaseLib", "PlayerSettings", "MenuManager"]

ignore = ["**/runtimes/win/**", "__MACOSX/**"]

[source]
type  = "github_release"
repo  = "Nereziel/cs2-WeaponPaints"
asset = '^WeaponPaints\\.zip$'

[[layout]]
from = "WeaponPaints"
to   = "addons/counterstrikesharp/plugins/WeaponPaints"

[[layout]]
from = "gamedata"
to   = "addons/counterstrikesharp/gamedata"

[[template]]
path = "addons/counterstrikesharp/configs/plugins/WeaponPaints/WeaponPaints.json"
body = '''
{
  "DatabaseHost": "${db.host}",
  "DatabasePort": ${db.port},
  "DatabaseName": "${db.name}",
  "DatabaseUser": "${db.user}",
  "DatabasePassword": "${db.pass}",
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
'''
"""

MANIFEST_ANYBASELIB = """\
# AnyBaseLib — shared base library used by WeaponPaints.

[source]
type  = "github_release"
repo  = "NickFox007/AnyBaseLibCS2"
asset = '^AnyBaseLib\\.zip$'
"""

MANIFEST_PLAYERSETTINGS = """\
# PlayerSettings — per-player settings store used by WeaponPaints.

[source]
type  = "github_release"
repo  = "NickFox007/PlayerSettingsCS2"
asset = '^PlayerSettings\\.zip$'

[[template]]
path = "addons/counterstrikesharp/configs/plugins/PlayerSettings/PlayerSettings.json"
body = '''
{
  "DatabaseParams": {
    "Host": "${db.host}:${db.port}",
    "Name": "${db.name}",
    "User": "${db.user}",
    "Password": "${db.pass}",
    "Table": "settings_"
  },
  "ConfigVersion": 1
}
'''
"""

MANIFEST_MENUMANAGER = """\
# MenuManager — in-game menu framework used by WeaponPaints.

[source]
type  = "github_release"
repo  = "NickFox007/MenuManagerCS2"
asset = '^MenuManager\\.zip$'
"""

MANIFESTS = {
    "WeaponPaints": MANIFEST_WEAPONPAINTS,
    "AnyBaseLib": MANIFEST_ANYBASELIB,
    "PlayerSettings": MANIFEST_PLAYERSETTINGS,
    "MenuManager": MANIFEST_MENUMANAGER,
}

# ---------------------------------------------------------------------------
# Tiny API client + assertion tracking (stdlib only)
# ---------------------------------------------------------------------------

_failures = []


def api(method, path, body=None, quiet=False):
    """Call the API. Returns (status, parsed_body). Never raises on HTTP errors."""
    url = BASE + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method)
    if data is not None:
        req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            status, raw = resp.status, resp.read().decode()
    except urllib.error.HTTPError as e:
        status, raw = e.code, e.read().decode()
    except urllib.error.URLError as e:
        return 0, f"request failed: {e}"
    payload = None
    if raw:
        try:
            payload = json.loads(raw)
        except json.JSONDecodeError:
            payload = raw
    if not quiet:
        print(f"    {method} {path} -> {status}")
        if status >= 400:
            print(f"      {payload}")
    return status, payload


def check(cond, msg):
    tag = "PASS" if cond else "FAIL"
    print(f"    [{tag}] {msg}")
    if not cond:
        _failures.append(msg)


def wait(secs, why=""):
    note = f" ({why})" if why else ""
    print(f"  ... waiting {secs}s{note}")
    time.sleep(secs)


# ---------------------------------------------------------------------------
# Pretty printers
# ---------------------------------------------------------------------------

def show_servers():
    _, servers = api("GET", "/api/servers", quiet=True)
    print("    servers:")
    for s in servers or []:
        loc = f"port {s['port']}" if s.get("port") else f"cluster {s['cluster']}"
        print(f"      - {s['name']:<10} {loc:<16} "
              f"desired={s['desired_state']:<8} actual={s['actual_state']:<9} ip={s['ip']}")
    return {s["name"]: s for s in servers or []}


def show_clusters():
    _, clusters = api("GET", "/api/clusters", quiet=True)
    print("    clusters:")
    for c in clusters or []:
        print(f"      - {c['name']:<10} port {c['port']:<8} "
              f"lb={c['lb_policy']:<12}")
    return {c["name"]: c for c in clusters or []}


def server_set(name, kind):
    """Read a server's own (server-scope) plugin/config set: {overridden, items}."""
    _, body = api("GET", f"/api/servers/{name}/{kind}", quiet=True)
    return body or {}


def cluster_set(name, kind):
    _, body = api("GET", f"/api/clusters/{name}/{kind}", quiet=True)
    return body or {}


# ---------------------------------------------------------------------------
# SSE listener (background thread)
# ---------------------------------------------------------------------------

def sse_listener(stop):
    try:
        with urllib.request.urlopen(BASE + "/api/events", timeout=120) as resp:
            buf = []
            for line in resp:
                if stop.is_set():
                    return
                line = line.decode().rstrip("\n")
                if line.startswith("data:"):
                    buf.append(line[5:].strip())
                elif line == "" and buf:
                    try:
                        servers = json.loads("".join(buf))
                        summary = ", ".join(
                            f"{s['name']}={s['actual_state']}" for s in servers
                        ) or "(no servers)"
                        print(f"  [sse {time.strftime('%H:%M:%S')}] {summary}")
                    except json.JSONDecodeError:
                        pass
                    buf = []
    except Exception as e:  # noqa: BLE001 - background thread, just report
        if not stop.is_set():
            print(f"  [sse] disconnected: {e}")


# ---------------------------------------------------------------------------
# Steps
# ---------------------------------------------------------------------------

def main():
    # Sanity: API reachable?
    status, _ = api("GET", "/api/servers", quiet=True)
    if status == 0:
        print(f"Cannot reach {BASE} — is the orchestrator running?")
        return

    stop = threading.Event()
    threading.Thread(target=sse_listener, args=(stop,), daemon=True).start()
    time.sleep(0.5)  # let SSE connect and print the initial snapshot

    print("\n== 1. create standalone server ==")
    status, created = api("POST", "/api/servers", {
        "name": STANDALONE,
        "port": STANDALONE_PORT,
        "accepting_connections": True,
        "restart_after_hrs": -1,
        "stop_after_hrs": -1,
    })
    check(status == 201, "standalone create returns 201")
    check(isinstance(created, dict) and created.get("ip", "").startswith("172.30.0."),
          f"ip was auto-allocated (got {created.get('ip') if isinstance(created, dict) else created})")

    wait(10, "standalone to start — watch SSE")

    print("\n== 2. list servers ==")
    servers = show_servers()
    check(STANDALONE in servers, f"{STANDALONE} present")
    check(servers.get(STANDALONE, {}).get("desired_state") == "running",
          f"{STANDALONE} desired_state == running")

    print("\n== 3. upload config + plugins, create cluster ==")
    s, _ = api("PUT", f"/api/configs/{CLUSTER_CONFIG}",
               {"filename": CLUSTER_CONFIG_FILENAME, "content": SKININSPECT_CFG})
    check(s == 204, f"upload {CLUSTER_CONFIG}")
    for pname in CLUSTER_PLUGINS:
        s, _ = api("PUT", f"/api/plugins/{pname}", {"manifest": MANIFESTS[pname]})
        check(s == 204, f"upload plugin {pname}")

    status, _ = api("POST", "/api/clusters", {
        "name": CLUSTER,
        "port": CLUSTER_PORT,
        "lb_policy": "round_robin",
        "plugins": CLUSTER_PLUGINS,
        "configs": [CLUSTER_CONFIG],
    })
    check(status == 201, "cluster create returns 201")

    show_clusters()
    cp = cluster_set(CLUSTER, "plugins")
    cc = cluster_set(CLUSTER, "configs")
    check(cp.get("overridden") and set(cp.get("items", [])) == set(CLUSTER_PLUGINS),
          f"cluster assigns all plugins {sorted(CLUSTER_PLUGINS)} (got {cp.get('items')})")
    check(cc.get("overridden") and cc.get("items") == [CLUSTER_CONFIG],
          f"cluster assigns config {CLUSTER_CONFIG} (got {cc.get('items')})")

    print("\n== 4. add member that inherits ==")
    status, _ = api("POST", "/api/servers", {
        "name": MEMBER_INHERIT,
        "cluster": CLUSTER,
    })
    check(status == 201, f"{MEMBER_INHERIT} create returns 201")
    wait(10, f"{MEMBER_INHERIT} to start")
    show_servers()
    show_clusters()
    ip_plugins = server_set(MEMBER_INHERIT, "plugins")
    ip_configs = server_set(MEMBER_INHERIT, "configs")
    check(ip_plugins.get("overridden") is False,
          f"{MEMBER_INHERIT} has no server-scope plugin override (inherits cluster)")
    check(ip_configs.get("overridden") is False,
          f"{MEMBER_INHERIT} has no server-scope config override (inherits cluster)")

    print("\n== 5. add member with explicitly NO plugins / NO configs ==")
    status, _ = api("POST", "/api/servers", {
        "name": MEMBER_NONE,
        "cluster": CLUSTER,
        "plugins": [],
        "configs": [],
    })
    check(status == 201, f"{MEMBER_NONE} create returns 201")
    wait(10, f"{MEMBER_NONE} to start")
    none_plugins = server_set(MEMBER_NONE, "plugins")
    none_configs = server_set(MEMBER_NONE, "configs")
    check(none_plugins.get("overridden") is True and none_plugins.get("items") in ([], None),
          f"{MEMBER_NONE} explicitly overrides plugins with none (got {none_plugins})")
    check(none_configs.get("overridden") is True and none_configs.get("items") in ([], None),
          f"{MEMBER_NONE} explicitly overrides configs with none (got {none_configs})")
    show_servers()
    show_clusters()

    # print("\n== 6. stop the standalone server ==")
    # s, _ = api("POST", f"/api/servers/{STANDALONE}/stop")
    # check(s == 204, f"stop {STANDALONE}")
    # wait(10, f"{STANDALONE} to drain")
    # servers = show_servers()
    # show_clusters()
    # check(servers.get(STANDALONE, {}).get("desired_state") == "stopped",
    #       f"{STANDALONE} desired_state == stopped")

    # print("\n== 7. stop the cluster (its members) ==")
    # # Clusters have no desired_state of their own, so "stopping the cluster" means
    # # stopping each member server.
    # for member in (MEMBER_INHERIT, MEMBER_NONE):
    #     s, _ = api("POST", f"/api/servers/{member}/stop")
    #     check(s == 204, f"stop {member}")
    # wait(10, "cluster members to drain")
    # servers = show_servers()
    # show_clusters()
    # check(all(servers.get(m, {}).get("desired_state") == "stopped"
    #           for m in (MEMBER_INHERIT, MEMBER_NONE)),
    #       "both cluster members desired_state == stopped")

    # # Bonus: env on a non-global scope is rejected (cluster/server env is create-only).
    # print("\n== bonus: cluster-scope env edit is rejected ==")
    # s, _ = api("PUT", "/api/env",
    #            {"key": "FOO", "value": "bar", "scope": "cluster", "scope_name": CLUSTER},
    #            quiet=True)
    # check(s == 400, f"PUT /api/env scope=cluster is rejected (got {s})")

    # print("\n== summary ==")
    # if _failures:
    #     print(f"  {len(_failures)} check(s) FAILED:")
    #     for f in _failures:
    #         print(f"    - {f}")
    # else:
    #     print("  all checks passed")

    # stop.set()


if __name__ == "__main__":
    main()
