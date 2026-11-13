//go:build !windows

package browsers

// Detect has nothing to look at off Windows.
func Detect() []Browser { return nil }
