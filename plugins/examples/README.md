# Plugin manifests

Each `*.toml` here is an example **recipe** for one CounterStrikeSharp / Metamod plugin: Where to fetch it, how to lay its files into the shared `base/` install, what it depends on, and how to template its config (including database).

Manifests are portable and written to the database via the GUI. A plugin's identity is its name, given during upload. That key is the primary key and is what other manifests' `requires` reference, so an entry in `requires` must match the key its dependency was uploaded under.

Credentials are resolved by the orchestrator at runtime based on the env variables the user inserts to the database. The same environment variables are propogated into the container as well.

The orchestrator reads the enabled plugins, resolves the dependency closure, fetches + lays everything into `base/game/csgo/`, and
rebuilds the whole `addons/` tree atomically when any version drifts.

## Schema

### Top level
- `requires` (array of names, optional) — other packages that must also be
  installed. Each name is a catalog key (see above). Used for closure ("enable
  WeaponPaints → pull its deps"). Load *order* is resolved by CounterStrikeSharp
  at runtime, so this is about inclusion, not install ordering.
- `ignore` (array of globs, optional) — archive entries to drop (junk dirs,
  wrong-OS binaries). Matched against archive-relative paths.

### `[source]` — where the bytes come from
- `type` — one of:
  - `github_release` — latest (or pinned) GitHub release asset.
  - `url` — a direct download URL.
  - `local` — absolute file path on the machine.
- `repo` — `"owner/name"` (for `github_release`).
- `asset` — regex selecting the asset by name (for `github_release`).
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
- `body` — multiline body where `${}` indicates a variable to be replaced by an env variable. 
- `path` — where the rendered file is written, relative to `game/csgo/`.

The following env variables are automatically injected from .env
- `${db.host}` from DB_HOST
- `${db.port}` from DB_PORT
- `${db.name}` from DB_NAME
- `${db.user}` from DB_USER
- `${db.pass}` from DB_PASS
- `$(db.rootpass)` from DB_ROOT_PASS

Other env variables can be added via the GUI.

This is whole-file templating. Some large auto-generated configs probably require a patch mode which we will add in the future. 

## Examples

Working plugin examples are given in this directory.
- CS-SimpleAdmin
- MenuManager
- AnyBaseLib
- cs2-retakes
- CS2Fixes
- SharpTimer
- WeaponPaints
