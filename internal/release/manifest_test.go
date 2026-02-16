package release

import "testing"

func TestParseAndResolveManifest(t *testing.T) {
	data := []byte("version=v1.2.3\nasset.linux.amd64.binary.name=maibot_linux_amd64\nasset.linux.amd64.binary.sha256=abcdef\nasset.linux.amd64.binary.url=https://example.com/maibot_linux_amd64\n")
	m, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("ParseManifest error: %v", err)
	}
	if m.Version != "v1.2.3" {
		t.Fatalf("version = %q", m.Version)
	}
	a, err := ResolveAsset(m, "linux", "amd64")
	if err != nil {
		t.Fatalf("ResolveAsset error: %v", err)
	}
	if a.Name != "maibot_linux_amd64" {
		t.Fatalf("asset name = %q", a.Name)
	}
}

func TestParseManifestMissingVersion(t *testing.T) {
	_, err := ParseManifest([]byte("asset.linux.amd64.binary.name=x\nasset.linux.amd64.binary.sha256=y\n"))
	if err == nil {
		t.Fatalf("expected missing version error")
	}
}
