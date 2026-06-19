# Plugin manifests

Each `*.toml` here is an example **recipe** for one CounterStrikeSharp / Metamod plugin:
where to fetch it, how to lay its files into the shared `base/` install, what it
depends on, and how to template its config (including database).

Manifests are portable and written to the database via the GUI: a manifest never contains secrets. Credentials are resolved by the orchestrator at runtime based on the env variables the user inserts to the database.

The orchestrator reads the enabled plugins, resolves the
dependency closure, fetches + lays everything into `base/game/csgo/`, and
rebuilds the whole `addons/` tree atomically when any version drifts.

## Schema

### Top level
- `name` (string, required) — unique package name. This is the primary key in the
  `plugin_manifests` database table and is referenced by other manifests' `requires`.
- `requires` (array of names, optional) — packages that must also be installed.
  Used for closure ("enable WeaponPaints → pull its deps"). Load *order* is
  resolved by CounterStrikeSharp at runtime, so this is about inclusion, not
  install ordering.
- `ignore` (array of globs, optional) — archive entries to drop (junk dirs,
  wrong-OS binaries). Matched against archive-relative paths.

### `[source]` — where the bytes come from
- `type` — one of:
  - `github_release` — latest (or pinned) GitHub release asset.
  - `url` — a direct download URL.
  - `allied_latest` — AlliedModders "latest" pointer file + base URL (MetaMod).
- `repo` — `"owner/name"` (for `github_release`).
- `asset` — regex selecting the asset by name (for `github_release`). Pick the
  *plugin-only* asset; avoid `with-cssharp` / `with-runtime` bundles that
  re-ship CounterStrikeSharp — we own that layer.
- `url` — direct URL (for `url`).
- `version` — `"latest"` (default) or a pinned release tag (for `github_release`).
- Archive format (`.zip` / `.tar.gz`) is autodetected from the asset name.

### `[[layout]]` — how extracted files map into the install
A list of copy rules. Each copies the **contents** of `from` into `to`:
- `from` — a glob, resolved against the archive root, matching one directory.
- `to`   — destination directory, relative to `game/csgo/`.

If no `[[layout]]` is given, the default is `from = "."`, `to = "."` — extract the
archive straight into `csgo/`. That covers every plugin whose archive is already
game-relative (`addons/...` at the root).

### `[[template]]` — config files rendered at boot
- `template` — template file in this directory (e.g. `weaponpaints.json.tmpl`).
- `path` — where the rendered file is written, relative to `game/csgo/`.

The orchestrator renders the template in the overlay fs, substituting
`${db.host}`, `${db.port}`, `${db.name}`, `${db.user}`, `${db.pass}` from the
the database credentials. Rendering stays on the overlay so secrets never touch the read-only `base/`.

This is **whole-file** templating: the `.tmpl` *is* the entire config, written
before launch so the plugin finds it already present. That works because we write
it pre-launch and is fine when the config is small enough to own (WeaponPaints,
InspectGive). For a large auto-generated config (e.g. CS2-SimpleAdmin, dozens of
unrelated settings), a whole-file template freezes all those other settings at the
version you copied. The eventual answer for those is a **patch** mode — declare
just the keys to set (e.g. `set = { "DatabaseConfig.DatabaseType" = "MySQL", ... }`)
and merge them into the plugin's own config — but mind the bootstrap order: the
plugin writes its config on first load, *after* launch, while the hook runs
*before* it, so there's nothing to patch on a clean boot. Whole-file is the
default for that reason.

## The shapes (taken from real plugins)

| shape | example | layout |
|---|---|---|
| A. game-relative (`addons/...` at root) | MenuManager, AnyBaseLib, cs2-retakes, CS2Fixes | default (none) |
| B. css-relative (`counterstrikesharp/...`) | CS2-SimpleAdmin | `from = "counterstrikesharp"` |
| C. named wrapper (`Zenith/...`) | K4-Zenith | `from = "Zenith"` |
| D. version-stamped wrapper | SharpTimer | `from = "SharpTimer-*"` (glob) |
| E. scattered (plugin dir + sibling dirs) | WeaponPaints | one rule per piece |

The manifests in this directory are the **live set** the orchestrator installs.
`examples/` holds hand-written manifests for plugins we don't install, kept as a
reference for each shape (asset regexes there may need tuning).
