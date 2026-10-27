package desktopdir

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

// ran makes dir a userData Desktop has run in, at t.
func ran(tb testing.TB, dir string, t time.Time) {
	tb.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		tb.Fatal(err)
	}
	p := filepath.Join(dir, "Local State")
	if err := os.WriteFile(p, []byte(`{}`), 0o644); err != nil {
		tb.Fatal(err)
	}
	if err := os.Chtimes(p, t, t); err != nil {
		tb.Fatal(err)
	}
}

// Desktop from the setup.exe that installs its MSIX package (Kilig on
// Discord, Windows 10 LTSC 21H2) keeps its data in the package's
// LocalCache, nothing in %APPDATA%\Claude: those are its folders, whatever
// the package's publisher id.
//
// The Store's package is the AnthropicPBC.Claude family
// (AnthropicPBC.Claude_fnn82j28hfe8t in Desktop 2.31226's own list).
func TestFindMSIX(t *testing.T) {
	for _, pub := range []string{"Claude_pzs8sxrjxfjjc", "Claude_q1w2e3r4t5y6z", "AnthropicPBC.Claude_fnn82j28hfe8t"} {
		home := t.TempDir()
		local, roaming := filepath.Join(home, "AppData", "Local"), filepath.Join(home, "AppData", "Roaming")
		pkg := filepath.Join(local, "Packages", pub, "LocalCache")
		ran(t, filepath.Join(pkg, "Roaming", "Claude"), time.Now())
		os.MkdirAll(filepath.Join(pkg, "Local", "Claude", "logs"), 0o755)
		os.MkdirAll(filepath.Join(local, "Packages", "Microsoft.WindowsStore_8wekyb3d8bbwe"), 0o755)
		os.MkdirAll(roaming, 0o755)

		got := Find("windows", home, env(map[string]string{"LOCALAPPDATA": local, "APPDATA": roaming}))
		want := Dirs{Data: filepath.Join(pkg, "Roaming", "Claude"), Mode: filepath.Join(pkg, "Local", "Claude"),
			ThreeP: filepath.Join(pkg, "Local", "Claude-3p"), Package: pkg}
		if got != want {
			t.Errorf("%s:\n got %+v\nwant %+v", pub, got, want)
		}
		if all := got.All(); !reflect.DeepEqual(all, []string{want.Data, want.Mode, want.ThreeP}) {
			t.Errorf("%s: all %v", pub, all)
		}
		// without the variables: the profile's own AppData
		if got := Find("windows", home, env(nil)); got != want {
			t.Errorf("%s, no variables: %+v", pub, got)
		}
	}
}

// The usual install (Squirrel: %APPDATA%\Claude, %LOCALAPPDATA%\Claude-3p)
// keeps its folders, and so does a machine with no Desktop at all. An
// %APPDATA%\Claude holding only claude_desktop_config.json (another tool's
// MCP servers) is not a Desktop that runs there; when both have run, the
// one that ran last is Desktop's.
func TestFindWindowsClassic(t *testing.T) {
	home := t.TempDir()
	local, roaming := filepath.Join(home, "Local"), filepath.Join(home, "Roaming")
	e := env(map[string]string{"LOCALAPPDATA": local, "APPDATA": roaming})
	classic := Dirs{Data: filepath.Join(roaming, "Claude"), Mode: filepath.Join(local, "Claude"), ThreeP: filepath.Join(local, "Claude-3p")}
	if got := Find("windows", home, e); got != classic {
		t.Errorf("nothing installed: %+v", got)
	}
	ran(t, filepath.Join(roaming, "Claude"), time.Now().Add(-time.Hour))
	if got := Find("windows", home, e); got != classic {
		t.Errorf("usual install: %+v", got)
	}

	pkg := filepath.Join(local, "Packages", "Claude_pzs8sxrjxfjjc", "LocalCache")
	os.MkdirAll(pkg, 0o755)
	if got := Find("windows", home, e); got != classic {
		t.Errorf("package never run: %+v", got)
	}
	ran(t, filepath.Join(pkg, "Roaming", "Claude"), time.Now().Add(-2*time.Hour))
	if got := Find("windows", home, e); got != classic {
		t.Errorf("package ran before the usual one: %+v", got)
	}
	ran(t, filepath.Join(pkg, "Roaming", "Claude"), time.Now())
	if got := Find("windows", home, e); got.Package != pkg || got.Data != filepath.Join(pkg, "Roaming", "Claude") {
		t.Errorf("package ran last: %+v", got)
	}

	// another tool's MCP file alone in %APPDATA%\Claude
	home2 := t.TempDir()
	local2, roaming2 := filepath.Join(home2, "Local"), filepath.Join(home2, "Roaming")
	os.MkdirAll(filepath.Join(roaming2, "Claude"), 0o755)
	os.WriteFile(filepath.Join(roaming2, "Claude", "claude_desktop_config.json"), []byte(`{"mcpServers":{}}`), 0o644)
	pkg2 := filepath.Join(local2, "Packages", "Claude_pzs8sxrjxfjjc", "LocalCache")
	ran(t, filepath.Join(pkg2, "Roaming", "Claude"), time.Now())
	if got := Find("windows", home2, env(map[string]string{"LOCALAPPDATA": local2, "APPDATA": roaming2})); got.Package != pkg2 {
		t.Errorf("only an MCP file in %%APPDATA%%\\Claude: %+v", got)
	}
}

// Desktop 3.x moved its userData out of the package into
// %LOCALAPPDATA%\Claude-Data\claude_<publisher>, leaving only a
// .user-data-left.<package> file in the package's folder. Its Cowork and
// Code sessions are in the new folder, so it is read as well (Shared), and
// the folder Desktop ran in counts when the package is ranked.
func TestFindSharedData(t *testing.T) {
	home := t.TempDir()
	local, roaming := filepath.Join(home, "AppData", "Local"), filepath.Join(home, "AppData", "Roaming")
	e := env(map[string]string{"LOCALAPPDATA": local, "APPDATA": roaming})
	pkg := filepath.Join(local, "Packages", "Claude_pzs8sxrjxfjjc", "LocalCache")
	old := filepath.Join(pkg, "Roaming", "Claude")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, ".user-data-left.claude_pzs8sxrjxfjjc"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(local, "Claude-Data", "claude_pzs8sxrjxfjjc")
	ran(t, shared, time.Now())

	got := Find("windows", home, e)
	if got.Package != pkg || got.Shared != shared {
		t.Fatalf("got %+v, want package %q and shared %q", got, pkg, shared)
	}
	if all := got.All(); !slices.Contains(all, shared) {
		t.Errorf("shared folder not read: %v", all)
	}
	if !slices.Contains(Sets("windows", home, e)[1].All(), shared) {
		t.Errorf("the other set misses the shared folder")
	}
	// it is a folder of its own, not Data or Mode
	if got.Data == shared || got.Mode == shared {
		t.Errorf("shared taken as Data or Mode: %+v", got)
	}
	// three mtimes: only the shared one is recent, and the package ran last
	ran(t, filepath.Join(roaming, "Claude"), time.Now().Add(-time.Hour))
	if got := Find("windows", home, e); got.Package != pkg {
		t.Errorf("classic ranked over the running package: %+v", got)
	}
}

// The folder is per Windows account, not per package: it is found with no
// package installed (the package removed after the move), and when several
// are there the one Desktop ran in last wins. Claude-Data is not taken for
// the deploymentMode folder (%LOCALAPPDATA%\Claude) either.
func TestFindSharedWithoutPackage(t *testing.T) {
	home := t.TempDir()
	local, roaming := filepath.Join(home, "Local"), filepath.Join(home, "Roaming")
	root := filepath.Join(local, "Claude-Data")
	older, newer := filepath.Join(root, "claude_a"), filepath.Join(root, "claude_b")
	ran(t, older, time.Now().Add(-time.Hour))
	ran(t, newer, time.Now())
	e := env(map[string]string{"LOCALAPPDATA": local, "APPDATA": roaming})
	got := Find("windows", home, e)
	if got.Shared != newer {
		t.Errorf("shared %q, want %q", got.Shared, newer)
	}
	if got.Package != "" {
		t.Errorf("package %q, want none", got.Package)
	}
	if want := filepath.Join(local, "Claude"); got.Mode != want {
		t.Errorf("mode %q, want %q", got.Mode, want)
	}

	// only the folder, no "Local State": still found
	home2 := t.TempDir()
	local2 := filepath.Join(home2, "Local")
	bare := filepath.Join(local2, "Claude-Data", "claude_c")
	if err := os.MkdirAll(bare, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := Find("windows", home2, env(map[string]string{"LOCALAPPDATA": local2})); got.Shared != bare {
		t.Errorf("bare folder: %+v", got)
	}
}

// macOS and Linux have one Claude folder for both, and no package.
func TestFindOthers(t *testing.T) {
	home := t.TempDir()
	as := filepath.Join(home, "Library", "Application Support")
	if got := Find("darwin", home, env(nil)); got != (Dirs{Data: filepath.Join(as, "Claude"), Mode: filepath.Join(as, "Claude"), ThreeP: filepath.Join(as, "Claude-3p")}) {
		t.Errorf("darwin: %+v", got)
	} else if len(got.All()) != 2 {
		t.Errorf("darwin all: %v", got.All())
	}
	for _, x := range []string{"", "relative/dir"} {
		c := filepath.Join(home, ".config")
		if got := Find("linux", home, env(map[string]string{"XDG_CONFIG_HOME": x})); got != (Dirs{Data: filepath.Join(c, "Claude"), Mode: filepath.Join(c, "Claude"), ThreeP: filepath.Join(c, "Claude-3p")}) {
			t.Errorf("linux %q: %+v", x, got)
		}
	}
}
