package provision

import (
	"encoding/json"
	"os"
)

// receipt records what we actually installed into base/ so
// on next startup we know the state of the files
type receipt struct {
	Game      gameReceipt   `json:"game"`
	ModBundle modBundle     `json:"modBundle"`
	Plugin    pluginReceipt `json:"plugin"`
	Skins     skinsReceipt  `json:"skins"`
}

type gameReceipt struct {
	BuildID string `json:"buildid"` // appmanifest_730.acf StateFlags-4 buildid
}

type modBundle struct {
	MetaMod      string            `json:"metamod"`      // mmsource-latest-linux filename
	CSS          string            `json:"css"`          // GitHub release tag
	WeaponPaints string            `json:"weaponpaints"` // GitHub release tag
	Deps         map[string]string `json:"deps"`         // dep name -> GitHub release tag
}

type pluginReceipt struct {
	SourceHash      string `json:"sourceHash"`      // hash of InspectGive/ source tree
	BuiltAgainstCSS string `json:"builtAgainstCss"` // CSS tag the plugin was built against
}

type skinsReceipt struct {
	Commit string `json:"commit"` // CSGO-API skins.json commit SHA
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
	return atomicWrite(path, out)
}
