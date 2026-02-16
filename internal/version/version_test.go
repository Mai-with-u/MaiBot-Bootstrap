package version

import (
	"strings"
	"testing"
)

func TestInstallerVersionNotEmpty(t *testing.T) {
	if strings.TrimSpace(InstallerVersion) == "" {
		t.Fatalf("InstallerVersion is empty")
	}
}
