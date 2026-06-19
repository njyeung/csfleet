package provision

import (
	"encoding/json"
	"os"

	"csfleet/orchestrator/internal/install"
)

// receipt records what we actually installed into base/ so
// on next startup we know the state of the files
type receipt struct {
	Game      gameReceipt `json:"game"`
	ModBundle modBundle   `json:"modBundle"`
}

type gameReceipt struct {
	BuildID string `json:"buildid"` // appmanifest_730.acf StateFlags-4 buildid
}

// modBundle is the shared base mod layer: only MetaMod + CounterStrikeSharp.
// Actual plugins (WeaponPaints, InspectGive, ...) are inserted per-instance into
// each server's overlay, not baked into base — see ApplyManifest.
type modBundle struct {
	MetaMod string `json:"metamod"` // mmsource-latest-linux filename
	CSS     string `json:"css"`     // GitHub release tag
}

// loadReceipt reads the receipt; a missing or corrupt file yields the zero
// value, which makes every step treat its component as "not installed".
func loadReceipt(path string) receipt {
	var r receipt
	data, err := os.ReadFile(path)
	if err != nil {
		return r
	}
	_ = json.Unmarshal(data, &r)
	return r
}

func saveReceipt(path string, r receipt) error {
	out, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return install.AtomicWrite(path, out)
}
