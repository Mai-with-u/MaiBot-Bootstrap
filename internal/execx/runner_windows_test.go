//go:build windows

package execx

import "testing"

func TestIsRootWindowsEnvOverride(t *testing.T) {
	t.Setenv("MAIBOT_ASSUME_ADMIN", "1")
	if !isRoot() {
		t.Fatalf("expected isRoot true when override set")
	}
}
