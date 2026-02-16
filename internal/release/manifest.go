package release

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const userAgent = "maibot-cli"

type Asset struct {
	Name   string
	URL    string
	SHA256 string
	SigURL string
}

type Manifest struct {
	Version          string
	InstallerVersion string
	Assets           map[string]Asset
}

func FetchManifest(repo, version string) (Manifest, error) {
	manifestURL := fmt.Sprintf("https://github.com/%s/releases/download/%s/manifest.txt", repo, version)
	body, err := fetch(manifestURL)
	if err != nil {
		return Manifest{}, err
	}
	manifest, err := ParseManifest(body)
	if err != nil {
		return Manifest{}, err
	}
	if manifest.Version == "" {
		manifest.Version = version
	}
	return manifest, nil
}

func ParseManifest(data []byte) (Manifest, error) {
	manifest := Manifest{Assets: map[string]Asset{}}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return Manifest{}, fmt.Errorf("invalid manifest line %d", lineNo)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "version":
			manifest.Version = value
		case "installer_version":
			manifest.InstallerVersion = value
		default:
			if err := applyAssetKV(&manifest, key, value); err != nil {
				return Manifest{}, err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Manifest{}, err
	}

	if manifest.Version == "" {
		return Manifest{}, errors.New("manifest missing version")
	}
	if len(manifest.Assets) == 0 {
		return Manifest{}, errors.New("manifest has no assets")
	}
	for k, a := range manifest.Assets {
		if a.Name == "" || a.SHA256 == "" {
			return Manifest{}, fmt.Errorf("manifest asset %s incomplete", k)
		}
	}

	return manifest, nil
}

func ResolveAsset(manifest Manifest, goos, goarch string) (Asset, error) {
	key := platformKey(goos, goarch)
	asset, ok := manifest.Assets[key]
	if !ok {
		return Asset{}, fmt.Errorf("manifest missing asset for %s", key)
	}
	return asset, nil
}

func platformKey(goos, goarch string) string {
	return goos + "." + goarch
}

func applyAssetKV(manifest *Manifest, key, value string) error {
	if !strings.HasPrefix(key, "asset.") {
		return nil
	}
	parts := strings.Split(key, ".")
	if len(parts) != 5 {
		return fmt.Errorf("invalid asset key: %s", key)
	}
	goos := parts[1]
	goarch := parts[2]
	field := parts[4]
	if parts[3] != "binary" {
		return fmt.Errorf("unsupported asset namespace: %s", key)
	}

	k := platformKey(goos, goarch)
	asset := manifest.Assets[k]

	switch field {
	case "name":
		asset.Name = value
	case "url":
		asset.URL = value
	case "sha256":
		asset.SHA256 = strings.ToLower(value)
	case "sig_url":
		asset.SigURL = value
	default:
		return fmt.Errorf("invalid asset field in key: %s", key)
	}

	manifest.Assets[k] = asset
	return nil
}

func fetch(url string) ([]byte, error) {
	client := &http.Client{Timeout: 45 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("request failed %s: %s - %s", url, resp.Status, strings.TrimSpace(string(body)))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}
