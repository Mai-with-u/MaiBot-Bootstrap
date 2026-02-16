package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type migrationStep struct {
	from int
	to   int
	run  func(Config, string) (Config, error)
}

var migrationPlan = []migrationStep{
	{from: 1, to: 2, run: migrateV1ToV2},
	{from: 2, to: 3, run: migrateV2ToV3},
}

func migrate(cfg Config, base string) (Config, error) {
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if cfg.Version > schemaVersion {
		return Config{}, fmt.Errorf("config version %d is newer than supported %d", cfg.Version, schemaVersion)
	}
	if cfg.Version == schemaVersion {
		return cfg, nil
	}

	backupPath := filepath.Join(base, "config.backup."+time.Now().UTC().Format("20060102-150405")+".json")
	if err := backupConfig(cfg, backupPath); err != nil {
		return Config{}, err
	}

	for cfg.Version < schemaVersion {
		step, ok := findStep(cfg.Version)
		if !ok {
			return Config{}, fmt.Errorf("missing migration step for version %d", cfg.Version)
		}
		next, err := step.run(cfg, base)
		if err != nil {
			return Config{}, err
		}
		cfg = next
		cfg.Version = step.to
	}
	return cfg, nil
}

func findStep(v int) (migrationStep, bool) {
	for _, s := range migrationPlan {
		if s.from == v {
			return s, true
		}
	}
	return migrationStep{}, false
}

func migrateV1ToV2(cfg Config, _ string) (Config, error) {
	if cfg.Installer.ReleaseChannel == "" {
		cfg.Installer.ReleaseChannel = "stable"
	}
	return cfg, nil
}

func migrateV2ToV3(cfg Config, _ string) (Config, error) {
	if cfg.Installer.Language == "" {
		cfg.Installer.Language = "auto"
	}
	return cfg, nil
}

func backupConfig(cfg Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
