// Shared client-side validation for the create/edit forms. These are
// user-friendliness hints to catch typos and mistakes early, NOT security
// boundaries — the API stays permissive on purpose, so someone hitting it
// directly can still do unusual things if they want to (e.g. a path that traverses upward).

// A server/cluster name becomes a Docker container name (`csfleet-<name>`, see
// server.go), a URL path segment, and the instance's identity, so it must be a
// safe identifier: an alphanumeric first character, then letters, digits, dot,
// dash or underscore. This rejects spaces and other unsafe characters up front
// instead of failing at container-create time.
const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9._-]*$/;

export function nameError(name: string): string | null {
	if (name === '') return null; // emptiness is enforced separately (required check)
	if (/\s/.test(name)) return 'No spaces allowed.';
	if (!NAME_RE.test(name)) return 'Use only letters, numbers, dot, dash or underscore.';
	return null;
}

// An env var key is injected into the container as `KEY=VALUE`, so it cannot
// contain whitespace or an `=`. A blank key is dropped on sync, not flagged.
export function isValidEnvKey(key: string): boolean {
	return key !== '' && !/\s/.test(key) && !key.includes('=');
}

// A plugin name is a single URL path segment (`/api/plugins/{name}`) and a DB
// key, so it follows the same safe-identifier rule as server/cluster names.
export const pluginNameError = nameError;

// A config's catalog name is path-like. It must not
// contain spaces or escape upward.
export function configNameError(name: string): string | null {
	if (name === '') return null;
	if (/\s/.test(name)) return 'No spaces allowed.';
	if (name.startsWith('/')) return 'Must not start with “/”.';
	if (name.split('/').includes('..')) return 'Must not contain “..”.';
	return null;
}

// A config filename is written to game/csgo/cfg/<filename> via filepath.Join
// with no traversal guard server-side (serverconfig/apply.go), so it must be a
// safe relative path: no spaces, no backslashes, not absolute, and no `..`
// segment that could escape cfg/.
export function filenameError(filename: string): string | null {
	if (filename === '') return null;
	if (/\s/.test(filename)) return 'No spaces allowed.';
	if (filename.includes('\\')) return 'Use “/” for subpaths, not “\\”.';
	if (filename.startsWith('/')) return 'Must be a relative path (no leading “/”).';
	if (filename.split('/').includes('..')) return 'Must not contain “..”.';
	return null;
}
