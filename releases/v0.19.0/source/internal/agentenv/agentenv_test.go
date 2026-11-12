package agentenv

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// unportedPkgs are the packages that read some of these variables upstream but
// are not part of AgentDeck.
//
// AgentDeck took magpie's session layer and left the model-routing gateway
// behind (see NOTICE.md), so agent, gateway, library and provider are gone and
// the variables only they read have no reader here. The names stay in Vars:
// the list is what a test sandbox clears, and a name cleared but never read
// costs nothing, where one dropped and still set by a developer's shell points
// a test at a real agent's folder (#522). So the check below is narrowed to the
// packages that are here, and a name whose only upstream reader is one of these
// is not a hole in this tree.
var unportedPkgs = map[string]bool{
	"agent": true, "gateway": true, "library": true, "provider": true,
	"backup": true, "plugin": true, "gui": true, "middleware": true,
}

// Every name in Vars is one magpie reads somewhere outside this package. A
// name it stopped reading, or one a sandbox invented, is a hole that every
// sandbox clearing this list believes is covered: internal/backup's cleared
// GEMINI_CLI_HOME and OPENCODE_CONFIG, which nothing reads, and not
// OPENCODE_CONFIG_DIR, which internal/agent does. This reads the sources
// rather than the agents' table, since the table is in internal/agent, which
// internal/sessions' tests cannot import; it looks for a quoted name, so a
// name only a comment mentions still counts, and a variable magpie reads
// through a name it builds at runtime would not.
func TestVarsAreRead(t *testing.T) {
	found := make(map[string]bool, len(Vars))
	err := filepath.WalkDir(filepath.Join("..", ".."), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// releases/ holds frozen copies of this tree (the source snapshot of
			// each release); counting a name read only there would say a name is
			// still read when the live tree has no reader for it.
			switch d.Name() {
			case ".git", "dist", "node_modules", "agentenv", "releases":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, v := range Vars {
			if !found[v] && strings.Contains(string(b), `"`+v+`"`) {
				found[v] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range Vars {
		if found[v] || unportedOnly(v) {
			continue
		}
		t.Errorf("%s is in Vars, but nothing outside this package reads it: magpie stopped reading it, or the name was never the one magpie reads", v)
	}
}

// upstreamReaders maps a variable to the packages that read it in magpie
// (main@63be477), for the names whose every reader AgentDeck left behind. It
// was generated from that tree and is checked in: the sources that would say so
// are exactly the ones that are gone.
var upstreamReaders = map[string][]string{
	"ALMA_DATA_DIR":                  {"agent"},
	"ANTE_HOME":                      {"agent"},
	"ASDF_DIR":                       {"agent"},
	"ASTRBOT_ROOT":                   {"agent"},
	"ATOMCODE_HOME":                  {"agent"},
	"CLAUDE_CODE_TMPDIR":             {"gateway"},
	"CLINE_MCP_SETTINGS_PATH":        {"library"},
	"CLOUDSDK_CONFIG":                {"provider"},
	"COPILOT_HOME":                   {"library", "provider"},
	"DEVECO_CONFIG_DIR":              {"agent"},
	"GOOGLE_APPLICATION_CREDENTIALS": {"provider"},
	"GROK_APP_HOME":                  {"agent"},
	"GROK_BIN_DIR":                   {"provider"},
	"JCODE_HOME":                     {"agent"},
	"KIMI_SHARE_DIR":                 {"agent"},
	"MAGPIE_ZED_BIN":                 {"agent"},
	"MAGPIE_ZED_CONFIG_DIR":          {"agent"},
	"MAGPIE_ZED_PROCESS_NAMES":       {"agent"},
	"MIMOCODE_HOME":                  {"agent"},
	"MISTER_MORPH_CONFIG":            {"agent"},
	"MOTHX_DIR":                      {"agent"},
	"OMO_CODING_AGENT_DIR":           {"agent"},
	"OMP_PROFILE":                    {"agent"},
	"OPENCHAMBER_DATA_DIR":           {"agent"},
	"OPENCODE_CONFIG_DIR":            {"agent"},
	"PI_CONFIG_DIR":                  {"agent"},
	"PI_PROFILE":                     {"agent"},
	"QWEN_HOME":                      {"agent"},
	"REASONIX_HOME":                  {"agent"},
	"REASONIX_STATE_HOME":            {"library"},
	"SENPI_CODING_AGENT_DIR":         {"agent"},
	"SNOW_CONFIG_DIR":                {"agent"},
	"STEPCODE_CONFIG_DIR":            {"agent"},
	"STEP_CODING_AGENT_DIR":          {"agent", "library"},
	"WINDSURF_API_SERVER_URL":        {"provider"},
	"ZCODE_CREDENTIAL_SECRET":        {"provider"},
}

// unportedOnly reports whether every package that reads v upstream is one
// AgentDeck did not take. Such a name has no reader here by design.
//
// The list is checked into the test rather than derived, because the sources
// that would say so are the ones that are gone. A name that gains a reader in
// the ported packages is still caught: found[v] short-circuits before this.
func unportedOnly(v string) bool {
	readers, ok := upstreamReaders[v]
	if !ok {
		return false
	}
	for _, r := range readers {
		if !unportedPkgs[r] {
			return false
		}
	}
	return true
}

// A name twice is a name a sandbox clears twice, which says the list was
// pasted together rather than kept.
func TestVarsAreApart(t *testing.T) {
	seen := make(map[string]bool, len(Vars))
	for _, v := range Vars {
		if seen[v] {
			t.Errorf("%s is in Vars twice", v)
		}
		seen[v] = true
	}
}
