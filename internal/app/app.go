package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"maibot/internal/config"
	"maibot/internal/logging"
	"maibot/internal/version"
)

const (
	instanceProc  = "run-single"
	defaultName   = "main"
	configVersion = 1
)

type App struct {
	cfg         config.Config
	log         *logging.Logger
	instanceLog *logging.Logger
	updateLog   *logging.Logger
	cleanupLog  *logging.Logger
}

func New() (*App, error) {
	cfg, err := config.LoadOrCreate()
	if err != nil {
		return nil, err
	}
	rootLog, err := logging.NewRoot(logging.Options{
		FilePath:       cfg.Logging.FilePath,
		MaxSizeMB:      cfg.Logging.MaxSizeMB,
		RetentionDays:  cfg.Logging.RetentionDays,
		MaxBackupFiles: cfg.Logging.MaxBackupFiles,
	})
	if err != nil {
		return nil, err
	}
	return &App{
		cfg:         cfg,
		log:         rootLog.Module("app"),
		instanceLog: rootLog.Module("instance"),
		updateLog:   rootLog.Module("update"),
		cleanupLog:  rootLog.Module("cleanup"),
	}, nil
}

func (a *App) Run(args []string) {
	if err := a.Execute(args); err != nil {
		a.log.Fatalf("command failed: %v", err)
	}
}

func (a *App) Execute(args []string) error {
	if err := a.validateConfig(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	cmd := a.newRootCommand()
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		return err
	}
	return nil
}

func (a *App) newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "maibot",
		Short: "MaiBot CLI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			a.runInteractiveTUI()
			return nil
		},
	}

	root.AddCommand(&cobra.Command{Use: "install", Aliases: []string{"create", "init"}, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.installInstance(defaultName); err != nil {
			return err
		}
		a.instanceLog.Okf("single workspace initialized")
		return nil
	}})

	root.AddCommand(&cobra.Command{Use: "start", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.startInstance(defaultName); err != nil {
			return err
		}
		a.instanceLog.Okf("workspace started")
		return nil
	}})

	root.AddCommand(&cobra.Command{Use: "stop", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.stopInstance(defaultName); err != nil {
			return err
		}
		a.instanceLog.Okf("workspace stopped")
		return nil
	}})

	root.AddCommand(&cobra.Command{Use: "restart", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.restartInstance(defaultName); err != nil {
			return err
		}
		a.instanceLog.Okf("workspace restarted")
		return nil
	}})

	root.AddCommand(&cobra.Command{Use: "status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return a.statusInstance(defaultName)
	}})

	logs := &cobra.Command{Use: "logs", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		tail, _ := cmd.Flags().GetInt("tail")
		return a.logsInstance(defaultName, tail)
	}}
	logs.Flags().Int("tail", 50, "Tail lines")
	root.AddCommand(logs)

	root.AddCommand(&cobra.Command{Use: "update", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.updateInstance(defaultName); err != nil {
			return err
		}
		a.updateLog.Okf("workspace updated")
		return nil
	}})

	root.AddCommand(&cobra.Command{Use: "self-update", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.selfUpdate(); err != nil {
			return err
		}
		a.updateLog.Okf("maibot updated successfully")
		return nil
	}})

	serviceCmd := &cobra.Command{Use: "service", Short: "Manage OS service for single workspace"}
	serviceCmd.AddCommand(&cobra.Command{Use: "install", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return a.serviceAction("install", defaultName)
	}})
	serviceCmd.AddCommand(&cobra.Command{Use: "uninstall", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return a.serviceAction("uninstall", defaultName)
	}})
	serviceCmd.AddCommand(&cobra.Command{Use: "start", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return a.serviceAction("start", defaultName)
	}})
	serviceCmd.AddCommand(&cobra.Command{Use: "stop", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return a.serviceAction("stop", defaultName)
	}})
	serviceCmd.AddCommand(&cobra.Command{Use: "status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return a.serviceAction("status", defaultName)
	}})
	root.AddCommand(serviceCmd)

	cleanup := &cobra.Command{Use: "cleanup", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		testArtifacts, _ := cmd.Flags().GetBool("test-artifacts")
		if !testArtifacts {
			return fmt.Errorf("usage: maibot cleanup --test-artifacts")
		}
		if err := a.cleanup(); err != nil {
			return err
		}
		a.cleanupLog.Okf("cleanup completed")
		return nil
	}}
	cleanup.Flags().Bool("test-artifacts", false, "Clean local test artifacts")
	root.AddCommand(cleanup)

	runCmd := &cobra.Command{Use: "run", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		sensitive, _ := cmd.Flags().GetBool("sensitive")
		sudo, _ := cmd.Flags().GetBool("sudo")
		prompt, _ := cmd.Flags().GetString("prompt")
		return a.runCommand(args, sensitive, sudo, prompt)
	}}
	runCmd.Flags().Bool("sensitive", false, "Require confirmation before running")
	runCmd.Flags().Bool("sudo", false, "Run command with sudo")
	runCmd.Flags().String("prompt", "", "Custom confirmation prompt")
	root.AddCommand(runCmd)

	root.AddCommand(&cobra.Command{Use: "version", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(version.InstallerVersion)
		return nil
	}})

	root.AddCommand(&cobra.Command{Use: instanceProc, Hidden: true, RunE: func(cmd *cobra.Command, args []string) error {
		id := workspaceID
		displayName := defaultName
		if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
			id = strings.TrimSpace(args[0])
		}
		if len(args) > 1 && strings.TrimSpace(args[1]) != "" {
			displayName = strings.TrimSpace(args[1])
		}
		a.runInstance(id, displayName)
		return nil
	}})

	return root
}

func (a *App) printHelp() {
	fmt.Println("MaiBot CLI")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  maibot install             Initialize single workspace")
	fmt.Println("  maibot create              Alias of install")
	fmt.Println("  maibot start               Start workspace")
	fmt.Println("  maibot stop                Stop workspace")
	fmt.Println("  maibot restart             Restart workspace")
	fmt.Println("  maibot status              Show workspace status")
	fmt.Println("  maibot logs [--tail N]     Show workspace logs")
	fmt.Println("  maibot update              Update workspace")
	fmt.Println("  maibot self-update         Update maibot command")
	fmt.Println("  maibot service <action>    Manage workspace service")
	fmt.Println("  maibot run <cmd...>        Run developer command")
	fmt.Println("  maibot cleanup --test-artifacts  Clean local test artifacts")
	fmt.Println("  maibot version             Print version")
}

func (a *App) validateConfig() error {
	if strings.TrimSpace(a.cfg.Installer.Repo) == "" {
		return errors.New("installer.repo is empty in config")
	}
	if strings.TrimSpace(a.cfg.Installer.ReleaseChannel) == "" {
		return errors.New("installer.release_channel is empty in config")
	}
	if strings.TrimSpace(a.cfg.Installer.DataHome) == "" {
		return errors.New("installer.data_home is empty in config")
	}
	if strings.TrimSpace(a.cfg.Installer.InstanceTickInterval) == "" {
		return errors.New("installer.instance_tick_interval is empty in config")
	}
	if a.cfg.Updater.RequireSignature && strings.TrimSpace(a.cfg.Updater.MiniSignPublicKey) == "" {
		return errors.New("updater.minisign_public_key is empty while signature is required")
	}
	return nil
}
