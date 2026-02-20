package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	koanfjson "github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const schemaVersion = 3

type Installer struct {
	Repo                 string `json:"repo"`
	ReleaseChannel       string `json:"release_channel"`
	Language             string `json:"language"`
	DataHome             string `json:"data_home"`
	InstanceTickInterval string `json:"instance_tick_interval"`
	LockTimeoutSeconds   int    `json:"lock_timeout_seconds"`
}

type Logging struct {
	FilePath       string `json:"file_path"`
	MaxSizeMB      int    `json:"max_size_mb"`
	RetentionDays  int    `json:"retention_days"`
	MaxBackupFiles int    `json:"max_backup_files"`
}

type Updater struct {
	RequireSignature  bool   `json:"require_signature"`
	MiniSignPublicKey string `json:"minisign_public_key"`
}

type Config struct {
	Version   int       `json:"version"`
	Installer Installer `json:"installer"`
	Logging   Logging   `json:"logging"`
	Updater   Updater   `json:"updater"`
}

func LoadOrCreate() (Config, error) {
	base, err := resolveBaseDir()
	if err != nil {
		return Config{}, err
	}
	path := filepath.Join(base, "maibot.conf")
	legacyPath := filepath.Join(base, "config.json")

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if _, legacyErr := os.Stat(legacyPath); legacyErr == nil {
			cfg, loadErr := loadFromPath(legacyPath)
			if loadErr != nil {
				return Config{}, loadErr
			}
			cfg, migrateErr := migrate(cfg, base)
			if migrateErr != nil {
				return Config{}, migrateErr
			}
			cfg = applyDefaults(cfg, base)
			if err := save(path, cfg); err != nil {
				return Config{}, err
			}
			return cfg, nil
		}
		cfg := defaults(base)
		if err := save(path, cfg); err != nil {
			return Config{}, err
		}
		return loadWithKoanf(path, base)
	}

	cfg, err := loadFromPath(path)
	if err != nil {
		return Config{}, err
	}
	cfg, err = migrate(cfg, base)
	if err != nil {
		return Config{}, err
	}
	cfg = applyDefaults(cfg, base)
	if err := save(path, cfg); err != nil {
		return Config{}, err
	}
	return loadWithKoanf(path, base)
}

func resolveBaseDir() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("MAIBOT_HOME")); explicit != "" {
		return explicit, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".maibot"), nil
}

func loadFromPath(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func loadWithKoanf(path, base string) (Config, error) {
	k := koanf.New(".")
	if err := k.Load(file.Provider(path), koanfjson.Parser()); err != nil {
		return Config{}, err
	}
	envPrefix := "MAIBOT_"
	if err := k.Load(env.Provider(envPrefix, ".", func(key string) string {
		n := strings.TrimPrefix(key, envPrefix)
		n = strings.ToLower(n)
		n = strings.ReplaceAll(n, "__", ".")
		return n
	}), nil); err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return Config{}, err
	}
	cfg = applyDefaults(cfg, base)
	return cfg, nil
}

func defaults(base string) Config {
	return Config{
		Version: schemaVersion,
		Installer: Installer{
			Repo:                 "Mai-with-u/maibot-bootstrap",
			ReleaseChannel:       "stable",
			Language:             "auto",
			DataHome:             base,
			InstanceTickInterval: "15s",
			LockTimeoutSeconds:   8,
		},
		Logging: Logging{
			FilePath:       filepath.Join(base, "logs", "installer.log"),
			MaxSizeMB:      10,
			RetentionDays:  7,
			MaxBackupFiles: 20,
		},
		Updater: Updater{
			RequireSignature:  false,
			MiniSignPublicKey: "",
		},
	}
}

func applyDefaults(cfg Config, base string) Config {
	d := defaults(base)
	if cfg.Version == 0 {
		cfg.Version = d.Version
	}
	if strings.TrimSpace(cfg.Installer.Repo) == "" {
		cfg.Installer.Repo = d.Installer.Repo
	}
	if strings.TrimSpace(cfg.Installer.ReleaseChannel) == "" {
		cfg.Installer.ReleaseChannel = d.Installer.ReleaseChannel
	}
	if strings.TrimSpace(cfg.Installer.Language) == "" {
		cfg.Installer.Language = d.Installer.Language
	}
	if strings.TrimSpace(cfg.Installer.DataHome) == "" {
		cfg.Installer.DataHome = d.Installer.DataHome
	}
	if strings.TrimSpace(cfg.Installer.InstanceTickInterval) == "" {
		cfg.Installer.InstanceTickInterval = d.Installer.InstanceTickInterval
	}
	if cfg.Installer.LockTimeoutSeconds <= 0 {
		cfg.Installer.LockTimeoutSeconds = d.Installer.LockTimeoutSeconds
	}
	if strings.TrimSpace(cfg.Logging.FilePath) == "" {
		cfg.Logging.FilePath = filepath.Join(cfg.Installer.DataHome, "logs", "installer.log")
	}
	if cfg.Logging.MaxSizeMB <= 0 {
		cfg.Logging.MaxSizeMB = d.Logging.MaxSizeMB
	}
	if cfg.Logging.RetentionDays <= 0 {
		cfg.Logging.RetentionDays = d.Logging.RetentionDays
	}
	if cfg.Logging.MaxBackupFiles <= 0 {
		cfg.Logging.MaxBackupFiles = d.Logging.MaxBackupFiles
	}
	if strings.TrimSpace(cfg.Updater.MiniSignPublicKey) == "" {
		cfg.Updater.MiniSignPublicKey = d.Updater.MiniSignPublicKey
	}
	return cfg
}

func save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.Installer.DataHome, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Logging.FilePath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
