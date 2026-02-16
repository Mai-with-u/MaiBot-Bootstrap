package app

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"maibot/internal/config"
	"maibot/internal/logging"
	"maibot/internal/version"
)

const (
	instanceProc  = "run-instance"
	defaultName   = "default"
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
		RunE: func(cmd *cobra.Command, args []string) error {
			a.runInteractiveTUI()
			return nil
		},
	}

	root.AddCommand(&cobra.Command{Use: "install [name]", Aliases: []string{"create"}, Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		name := resolveInstanceName(args)
		if err := a.installInstance(name); err != nil {
			return err
		}
		a.instanceLog.Okf("instance %q installed", name)
		return nil
	}})

	root.AddCommand(&cobra.Command{Use: "start [name]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		name := resolveInstanceName(args)
		if err := a.startInstance(name); err != nil {
			return err
		}
		a.instanceLog.Okf("instance %q started", name)
		return nil
	}})

	root.AddCommand(&cobra.Command{Use: "stop [name]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		name := resolveInstanceName(args)
		if err := a.stopInstance(name); err != nil {
			return err
		}
		a.instanceLog.Okf("instance %q stopped", name)
		return nil
	}})

	root.AddCommand(&cobra.Command{Use: "restart [name]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		name := resolveInstanceName(args)
		if err := a.restartInstance(name); err != nil {
			return err
		}
		a.instanceLog.Okf("instance %q restarted", name)
		return nil
	}})

	root.AddCommand(&cobra.Command{Use: "status [name]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return a.statusInstance(resolveInstanceName(args))
	}})

	logs := &cobra.Command{Use: "logs [name]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		tail, _ := cmd.Flags().GetInt("tail")
		return a.logsInstance(resolveInstanceName(args), tail)
	}}
	logs.Flags().Int("tail", 50, "Tail lines")
	root.AddCommand(logs)

	root.AddCommand(&cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return a.listInstances()
	}})

	root.AddCommand(&cobra.Command{Use: "update [name]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		name := resolveInstanceName(args)
		if err := a.updateInstance(name); err != nil {
			return err
		}
		a.updateLog.Okf("instance %q updated", name)
		return nil
	}})

	root.AddCommand(&cobra.Command{Use: "self-update", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if err := a.selfUpdate(); err != nil {
			return err
		}
		a.updateLog.Okf("maibot updated successfully")
		return nil
	}})

	serviceCmd := &cobra.Command{Use: "service", Short: "Manage OS service for an instance"}
	serviceCmd.AddCommand(&cobra.Command{Use: "install [name]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return a.serviceAction("install", resolveInstanceName(args))
	}})
	serviceCmd.AddCommand(&cobra.Command{Use: "uninstall [name]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return a.serviceAction("uninstall", resolveInstanceName(args))
	}})
	serviceCmd.AddCommand(&cobra.Command{Use: "start [name]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return a.serviceAction("start", resolveInstanceName(args))
	}})
	serviceCmd.AddCommand(&cobra.Command{Use: "stop [name]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return a.serviceAction("stop", resolveInstanceName(args))
	}})
	serviceCmd.AddCommand(&cobra.Command{Use: "status [name]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return a.serviceAction("status", resolveInstanceName(args))
	}})
	root.AddCommand(serviceCmd)

	cleanup := &cobra.Command{Use: "cleanup", Args: cobra.ArbitraryArgs, RunE: func(cmd *cobra.Command, args []string) error {
		testArtifacts, _ := cmd.Flags().GetBool("test-artifacts")
		if !testArtifacts {
			return fmt.Errorf("usage: maibot cleanup --test-artifacts [instance_names...]")
		}
		cleanArgs := append([]string{"--test-artifacts"}, args...)
		if err := a.cleanup(cleanArgs); err != nil {
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

	root.AddCommand(&cobra.Command{Use: "version", Aliases: []string{"--version", "-v"}, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(version.InstallerVersion)
		return nil
	}})

	root.AddCommand(&cobra.Command{Use: instanceProc, Hidden: true, RunE: func(cmd *cobra.Command, args []string) error {
		id := resolveInstanceName(args)
		displayName := id
		if len(args) > 1 {
			displayName = args[1]
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
	fmt.Println("  maibot install [name]      Install an instance")
	fmt.Println("  maibot create [name]       Alias of install")
	fmt.Println("  maibot start [name]        Start an instance")
	fmt.Println("  maibot stop [name]         Stop an instance")
	fmt.Println("  maibot restart [name]      Restart an instance")
	fmt.Println("  maibot status [name]       Show instance status")
	fmt.Println("  maibot logs [name] [--tail N]  Show instance logs")
	fmt.Println("  maibot update [name]       Update an instance")
	fmt.Println("  maibot self-update         Update maibot command")
	fmt.Println("  maibot list                List all instances")
	fmt.Println("  maibot cleanup --test-artifacts [names...]  Clean local test artifacts")
	fmt.Println("  maibot version             Print version")
}

func resolveInstanceName(args []string) string {
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return defaultName
	}
	return args[0]
}

func resolveLogsArgs(args []string) (name string, tail int) {
	name = defaultName
	tail = 50
	if len(args) == 0 {
		return name, tail
	}

	idx := 0
	if args[0] != "--tail" {
		name = args[0]
		if strings.TrimSpace(name) == "" {
			name = defaultName
		}
		idx = 1
	}
	if len(args) >= idx+2 && args[idx] == "--tail" {
		parsed := strings.TrimSpace(args[idx+1])
		if v, err := strconv.Atoi(parsed); err == nil && v > 0 {
			tail = v
		}
	}
	return name, tail
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
