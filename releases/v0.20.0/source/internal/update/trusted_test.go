package update

import "testing"

func TestTrustedAssetURL(t *testing.T) {
	cases := map[string]bool{
		"https://github.com/enpiaohj/agentdeck/releases/download/v1.0.0/AgentDeck-v1.0.0-win-x64.exe": true,
		"https://GitHub.com/x/y/releases/download/v1/a.exe":                                           true,
		"http://github.com/x/y/releases/download/v1/a.exe":                                            false,
		"https://github.com.evil.example/x/a.exe":                                                     false,
		"https://evil.example/github.com/a.exe":                                                       false,
		"https://user@github.com@evil.example/a.exe":                                                  false,
		"file:///C:/a.exe": false,
		"":                 false,
	}
	for raw, want := range cases {
		if got := trustedAssetURL(raw); got != want {
			t.Errorf("trustedAssetURL(%q) = %v，期望 %v", raw, got, want)
		}
	}
}
