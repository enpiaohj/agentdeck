package chatgpthistory

// ChatGPT's data export has no project names (only the `g-p-…` ids), but the
// ChatGPT/Codex desktop app keeps the list of the user's Projects in its own
// state file, ids and names together. Reading it lets the import page offer the
// real names. It is only ever read: nothing is written to it, and of everything
// in it only the three subtrees below are looked at.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	stateFile = ".codex-global-state.json"
	// maxState bounds the file read; the real one is a few hundred KB.
	maxState = 64 << 20
)

// LocalProjectNames returns project id → name as the desktop app in codexHome
// (Codex's folder: $CODEX_HOME, else ~/.codex) knows them. Missing, unreadable
// or unrecognised state gives an empty map: this is a convenience, never an
// error the import depends on.
func LocalProjectNames(codexHome string) map[string]string {
	out := map[string]string{}
	if codexHome == "" {
		return out
	}
	path := filepath.Join(codexHome, stateFile)
	if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() || info.Size() > maxState {
		return out
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var state struct {
		Atom  map[string]json.RawMessage `json:"electron-persisted-atom-state"`
		Local map[string]struct {
			Name string `json:"name"`
		} `json:"local-projects"`
	}
	if json.Unmarshal(data, &state) != nil {
		return out
	}
	put := func(id, name string) {
		if n := cleanName(name); strings.HasPrefix(id, "g-p-") && n != "" {
			out[id] = n
		}
	}
	// Later sources win: the older local-projects list, then the sidebar state of
	// each version in order (…-v1 before …-v2), so the newest name is kept.
	for id, p := range state.Local {
		put(id, p.Name)
	}
	var keys []string
	for k := range state.Atom {
		if strings.HasPrefix(k, "chatgpt-sidebar-state") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		var byAccount map[string]struct {
			Projects []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"projects"`
		}
		if json.Unmarshal(state.Atom[k], &byAccount) != nil {
			continue
		}
		for _, account := range byAccount {
			for _, p := range account.Projects {
				put(p.ID, p.Name)
			}
		}
	}
	return out
}

// ApplySuggestions fills in the name each project of res has in the desktop app.
func ApplySuggestions(res *PreviewResult, local map[string]string) {
	for i := range res.Projects {
		res.Projects[i].Suggested = local[res.Projects[i].ID]
	}
}
