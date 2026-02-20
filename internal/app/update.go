package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/google/go-github/v66/github"
	minisign "github.com/jedisct1/go-minisign"
	"maibot/internal/release"
	"maibot/internal/version"
)

const userAgent = "maibot-cli"

func (a *App) selfUpdate() error {
	current := strings.TrimSpace(version.InstallerVersion)
	target, err := a.latestVersion()
	if err != nil {
		return err
	}
	manifest, err := release.FetchManifest(a.cfg.Installer.Repo, target)
	if err != nil {
		return err
	}
	asset, err := release.ResolveAsset(manifest, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	if manifest.InstallerVersion != "" {
		target = manifest.InstallerVersion
	}
	if target == current {
		a.log.Infof(a.tf("log.already_latest", current))
		return nil
	}

	url := strings.TrimSpace(asset.URL)
	if url == "" {
		url = fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", a.cfg.Installer.Repo, manifest.Version, asset.Name)
	}

	a.updateLog.Infof(a.tf("log.downloading", url))
	binaryData, err := fetch(url)
	if err != nil {
		return err
	}
	want := strings.ToLower(strings.TrimSpace(asset.SHA256))
	have := sha256Hex(binaryData)
	if want != have {
		return fmt.Errorf("checksum mismatch for %s: want=%s have=%s", asset.Name, want, have)
	}

	if err := a.verifySignatureIfRequired(asset, url, binaryData); err != nil {
		return err
	}

	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	backupPath := exePath + ".bak"
	if err := os.WriteFile(exePath+".new", binaryData, 0o755); err != nil {
		return err
	}
	if err := os.Rename(exePath, backupPath); err != nil {
		_ = os.Remove(exePath + ".new")
		return err
	}
	if err := os.Rename(exePath+".new", exePath); err != nil {
		_ = os.Rename(backupPath, exePath)
		_ = os.Remove(exePath + ".new")
		return err
	}
	_ = os.Remove(backupPath)
	a.updateLog.Infof(a.tf("log.updated_from_to", current, target))
	return nil
}

func (a *App) latestVersion() (string, error) {
	owner, repo, err := splitRepo(a.cfg.Installer.Repo)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client := github.NewClient(nil)
	rel, _, err := client.Repositories.GetLatestRelease(ctx, owner, repo)
	if err != nil {
		return "", err
	}
	tag := strings.TrimSpace(rel.GetTagName())
	if tag == "" {
		return "", errors.New("latest release tag is empty")
	}
	return tag, nil
}

func (a *App) verifySignatureIfRequired(asset release.Asset, assetURL string, bin []byte) error {
	pubKey := strings.TrimSpace(a.cfg.Updater.MiniSignPublicKey)
	if !a.cfg.Updater.RequireSignature && pubKey == "" {
		return nil
	}
	if pubKey == "" {
		return errors.New("signature required but updater.minisign_public_key is empty")
	}
	sigURL := strings.TrimSpace(asset.SigURL)
	if sigURL == "" {
		sigURL = assetURL + ".minisig"
	}
	sigData, err := fetch(sigURL)
	if err != nil {
		if a.cfg.Updater.RequireSignature {
			return fmt.Errorf("failed to fetch signature: %w", err)
		}
		a.updateLog.Warnf(a.tf("log.signature_verify_skipped", err))
		return nil
	}

	pub, err := minisign.NewPublicKey(pubKey)
	if err != nil {
		return err
	}
	sig, err := minisign.DecodeSignature(string(sigData))
	if err != nil {
		return err
	}
	valid, err := pub.Verify(bin, sig)
	if err != nil {
		return err
	}
	if !valid {
		return errors.New("signature verification failed")
	}
	a.updateLog.Infof(a.t("log.signature_verified"))
	return nil
}

func splitRepo(repo string) (string, string, error) {
	parts := strings.Split(strings.TrimSpace(repo), "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("invalid repo format: %s", repo)
	}
	return parts[0], parts[1], nil
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
