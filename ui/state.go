package ui

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/google/renameio/v2"
)

type state struct {
	Search  string `json:"search"`
	Country string `json:"country,omitempty"`
}

func statePath() string {
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "delayfm", "state.json")
}

func loadState() state {
	data, err := os.ReadFile(statePath())
	if err != nil {
		return state{}
	}

	var s state
	_ = json.Unmarshal(data, &s)
	return s
}

func saveState(s state) {
	path := statePath()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)

	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = renameio.WriteFile(path, data, 0o644)
}
