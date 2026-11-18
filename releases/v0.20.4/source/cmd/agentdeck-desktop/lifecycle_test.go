package main

import "testing"

func TestActivationWaitsForApplicationStartup(t *testing.T) {
	calls := 0
	g := activationGate{dispatch: func() { calls++ }}
	g.request()
	g.request()
	if calls != 0 {
		t.Fatal("activation dispatched before startup")
	}
	g.start()
	if calls != 1 {
		t.Fatal("early activation was not preserved")
	}
	g.request()
	if calls != 2 {
		t.Fatal("running application did not activate")
	}
	g.stop()
	g.request()
	g.start()
	if calls != 2 {
		t.Fatal("activation dispatched after shutdown")
	}
}

func TestCloseToTrayAndExplicitQuit(t *testing.T) {
	enabled, hidden, quit := false, 0, 0
	l := desktopLifecycle{closeToTray: func() bool { return enabled }, hide: func() { hidden++ }, quit: func() { quit++ }}
	if l.closeRequested() || hidden != 0 {
		t.Fatal("default close must exit normally")
	}
	enabled = true
	if !l.closeRequested() || hidden != 1 {
		t.Fatal("enabled close must hide")
	}
	l.exit()
	l.exit()
	if quit != 1 || l.closeRequested() || hidden != 1 {
		t.Fatal("explicit quit must bypass close interception exactly once")
	}
}

func TestStartupHidden(t *testing.T) {
	for _, tc := range []struct {
		args       []string
		tray, want bool
	}{
		{nil, true, false}, {[]string{"--startup"}, true, true},
		{[]string{"--startup"}, false, false}, {[]string{"--relaunch"}, true, false},
		{[]string{"--startup=false"}, true, false},
	} {
		if got := startupHidden(tc.args, tc.tray); got != tc.want {
			t.Errorf("%v tray=%v got %v", tc.args, tc.tray, got)
		}
	}
}
