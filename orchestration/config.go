package main

// Hardcoded orchestrator config. This is deliberately a plain struct literal for
// now: eventually a frontend (TUI or web) will drive these — per-server env,
// configs, and the port range — but until then we edit them here.
//
// Secrets live in .env for the docker-compose path; mirrored here so the Go
// orchestrator is self-contained. Fine for local/dev use.

type Config struct {
	// Shared DB the WeaponPaints/InspectGive plugins talk to. Reachable from the
	// server containers by this name on the shared docker network.
	DBHost string
	DBPort int
	DBName string
	DBUser string
	DBPass string

	// GSLT token (CS2_LAN=0 -> internet-visible). Empty + LAN=1 for LAN-only.
	SrcdsToken string
	LAN        bool
	RconPW     string
	ServerPW   string

	// Docker network every container (servers + db) joins so they can resolve
	// each other by name. The orchestrator expects it to already exist.
	Network string

	// Port pool. Each server claims one game port; SourceTV uses port+GOTVOffset.
	PortStart  int
	PortEnd    int
	GOTVOffset int

	// The servers to run. Every server shares the same cvar config
	// (config/skininspect.cfg); they differ only in name + map for now.
	Servers []ServerSpec
}

type ServerSpec struct {
	Name string // container/instance name suffix, e.g. "dust2"
	Map  string // CS2_STARTMAP, e.g. "de_dust2"
}

func defaultConfig() Config {
	return Config{
		DBHost:     "cs2-mariadb",
		DBPort:     3306,
		DBName:     "weaponpaints",
		DBUser:     "weaponpaints",
		DBPass:     "dbpass",
		SrcdsToken: "832E2CDA3F227734BD3B661CBD95A162",
		LAN:        false,
		RconPW:     "cs2rconpw",
		ServerPW:   "",
		Network:    "csfleet",
		PortStart:  27015,
		PortEnd:    27045,
		GOTVOffset: 5,
		Servers: []ServerSpec{
			{Name: "dust2", Map: "de_dust2"},
			{Name: "mirage", Map: "de_mirage"},
			{Name: "nuke", Map: "de_nuke"},
		},
	}
}
