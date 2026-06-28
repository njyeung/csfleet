// CS2 env catalog — frontend reference data mirroring the `joedwards32/cs2`
// image and the orchestrator's reserved vars. This is
// suggestion/help data only; arbitrary keys are still allowed in the editor.
//
// Keep in sync with the image and orchestration/server/server.go /
// orchestration/fleet/worker.go (like types.ts ↔ types.go).

export interface CatalogVar {
	key: string;
	// Suggested default, prefilled when added. Empty string = "usually left blank".
	default: string;
	description: string;
}

export interface CatalogGroup {
	label: string;
	vars: CatalogVar[];
}

// Grouped suggestions offered by "+ Add from CS2 catalog". `CS2_SERVERNAME`'s
// real default is the instance name (filled by the editor), so it's '' here.
export const CS2_CATALOG: CatalogGroup[] = [
	{
		label: 'Server',
		vars: [
			{ key: 'CS2_SERVERNAME', default: '', description: 'Server name in the browser. Defaults to the instance name.' },
			{ key: 'CS2_CHEATS', default: '0', description: 'Enable sv_cheats (0 = off).' },
			{ key: 'CS2_SERVER_HIBERNATE', default: '0', description: 'Hibernate when empty (0 = off).' },
			{ key: 'CS2_LAN', default: '0', description: 'LAN mode (0 = off).' },
			{ key: 'CS2_RCONPW', default: 'changeme', description: 'RCON password.' },
			{ key: 'CS2_PW', default: '', description: 'Server join password (blank = public).' },
			{ key: 'CS2_MAXPLAYERS', default: '10', description: 'Maximum player slots.' },
			{ key: 'CS2_ADDITIONAL_ARGS', default: '', description: 'Extra args appended to the launch command.' }
		]
	},
	{
		label: 'Game modes',
		vars: [
			{ key: 'CS2_GAMEALIAS', default: '', description: 'Game mode alias (e.g. competitive, casual, deathmatch).' },
			{ key: 'CS2_GAMETYPE', default: '0', description: 'game_type value.' },
			{ key: 'CS2_GAMEMODE', default: '1', description: 'game_mode value.' },
			{ key: 'CS2_MAPGROUP', default: 'mg_active', description: 'Map group.' },
			{ key: 'CS2_STARTMAP', default: 'de_inferno', description: 'Starting map.' }
		]
	},
	{
		label: 'Bots',
		vars: [
			{ key: 'CS2_BOT_DIFFICULTY', default: '', description: 'Bot difficulty (0–3).' },
			{ key: 'CS2_BOT_QUOTA', default: '', description: 'Number of bots.' },
			{ key: 'CS2_BOT_QUOTA_MODE', default: '', description: 'Bot quota mode (fill, competitive, normal).' }
		]
	},
	{
		label: 'CSTV',
		vars: [
			{ key: 'TV_ENABLE', default: '0', description: 'Enable GOTV (0 = off).' },
			{ key: 'TV_AUTORECORD', default: '0', description: 'Auto-record demos (0 = off).' },
			{ key: 'TV_PW', default: 'changeme', description: 'GOTV spectator password.' },
			{ key: 'TV_RELAY_PW', default: 'changeme', description: 'GOTV relay password.' },
			{ key: 'TV_MAXRATE', default: '0', description: 'GOTV max rate (0 = unlimited).' },
			{ key: 'TV_DELAY', default: '0', description: 'GOTV broadcast delay (seconds).' }
		]
	},
	{
		label: 'Logs',
		vars: [
			{ key: 'CS2_LOG', default: 'on', description: 'Enable logging.' },
			{ key: 'CS2_LOG_MONEY', default: '0', description: 'Log money changes.' },
			{ key: 'CS2_LOG_DETAIL', default: '0', description: 'Log detail level.' },
			{ key: 'CS2_LOG_ITEMS', default: '0', description: 'Log item pickups.' },
			{ key: 'CS2_LOG_FILE', default: '0', description: 'Log to file.' },
			{ key: 'CS2_LOG_ECHO', default: '0', description: 'Echo logs to console.' },
			{ key: 'CS2_DISCONNECT_KILLS', default: '1', description: 'Kill a player’s pawn on disconnect.' },
			{ key: 'CS2_LOG_HTTP_URL', default: '', description: 'POST logs to this HTTP endpoint.' }
		]
	},
	{
		label: 'Workshop',
		vars: [
			{ key: 'CS2_HOST_WORKSHOP_MAP', default: '', description: 'Workshop map ID to host.' },
			{ key: 'CS2_HOST_WORKSHOP_COLLECTION', default: '', description: 'Workshop collection ID.' }
		]
	}
];

// Reserved vars — set by the orchestrator at container start, so a
// user value is ignored/harmful. The editor silently drops these (no-op). The
// GSLT token (SRCDS_TOKEN) is handled by the dedicated TokenField, not here.
export const RESERVED_VARS = new Set(['CS2_PORT', 'CS2_RCON_PORT', 'TV_PORT', 'CS2_IP', 'SRCDS_TOKEN']);

export const isReserved = (key: string) => RESERVED_VARS.has(key.trim());
